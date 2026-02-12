// Package sidecar implements a transport layer that communicates with a local sidecar process.
// It uses a combination of Unix domain sockets for control messages (registration, heartbeats)
// and shared memory for high-performance data exchange of transport packages.
package sidecar

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/linchenxuan/strix/log"
	"github.com/linchenxuan/strix/metrics"
	"github.com/linchenxuan/strix/network/message"
	"github.com/linchenxuan/strix/network/pb"
	"github.com/linchenxuan/strix/network/transport"
	"github.com/linchenxuan/strix/runtime"
	"github.com/linchenxuan/strix/utils/file"
	"github.com/linchenxuan/strix/utils/shm"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	// FactoryName is the name of the sidecar plugin, used for registration in the transport factory.
	FactoryName = "sidecartransport"

	// _waitFileTimeout is the timeout for waiting for a file to appear.
	_waitFileTimeout = time.Second * 5
	// _waitFileSleepTime is the sleep interval when waiting for a file.
	_waitFileSleepTime = time.Millisecond * 100
)

// Constants for sidecar communication protocols.
const (
	// _maxMsgSize is the maximum size of a message that can be processed.
	_maxMsgSize = 10 * 1024 * 1024

	// Unix Domain Socket related constants
	_unixSockHdrSize         = 16  // Size of the header for messages on the Unix socket.
	_unixSockReadTimeoutSec  = 3   // Read timeout for the Unix socket.
	_unixSockWriteTimeoutSec = 3   // Write timeout for the Unix socket.
	_unixSockUpdateInternMS  = 100 // Interval for the main loop when the connection is healthy.
	_unixSockMsgBuffSize     = 256 // Initial buffer size for Unix socket messages.

	// Shared Memory (SHM) related constants
	_shmHdrSize        = 30                 // Size of the shared memory message header.
	_shmMsgValidCode   = 0xefeffefeefeffefe // Magic code to validate a message in shared memory.
	_shmVersion        = 1                  // Version of the shared memory protocol.
	_shmCtrlHdrSize    = 1024               // Size of the control header in the shared memory region.
	_shmMaxValidMsgSec = 6                  // Maximum validity duration for a message in shared memory.

	// Recv loop related constants
	_recvIdleCntToSleep = 5                    // Number of idle recv calls before sleeping to reduce CPU usage.
	_recvSleepTime      = 2 * time.Millisecond // Sleep duration during idle recv periods.

	// General constants
	_stopTimeout            = 2 * time.Second // Timeout for stopping the transport.
	_baseShmpipeIdxMetaName = "ShmPipeIdx"    // Base name for the pipe index in message metadata.
)

// sidecarState represents the connection state of the sidecar transport.
type sidecarState int

const (
	_stateNone       sidecarState = iota // Initial state, not yet initialized.
	_stateInited                         // Initialized, but not yet connected.
	_stateConnecting                     // Attempting to connect and register with the sidecar.
	_stateSucceed                        // Successfully connected and registered.
	_stateStopped                        // The transport has been stopped.
	_stateMigrate                        // A migration process has been initiated.
)

// sidecarMsg wraps a protobuf message to be sent over the Unix socket.
type sidecarMsg struct {
	id  int
	msg protoreflect.ProtoMessage
}

// Sidecar implements the transport.SSTransport interface for sidecar-based communication.
type Sidecar struct {
	state              sidecarState                 // Current state of the sidecar connection.
	svrState           int32                        // Current state of the server (e.g., running, exiting).
	cfg                *SidecarConfig               // Configuration for the sidecar transport.
	ss                 shm.ShmpipeSelector          // Selector for managing shared memory pipe locks.
	stopRecv           chan bool                    // Channel to signal the recv loop to stop.
	closeUnixSocket    chan struct{}                // Channel to manually trigger closing the Unix socket, e.g., for migration.
	version            int64                        // Version number obtained from the sidecar upon registration.
	shmpipeIdx         int                          // The index of the shared memory pipe this instance is using.
	shmpipeIdxMetaName string                       // The key used in message metadata to store the pipe index.
	shmChannel         shmChannel                   // The shared memory channel for reading and writing data.
	main               bool                         // Whether this instance is the main service instance (determined by sidecar).
	conn               *net.UnixConn                // The Unix domain socket connection to the sidecar.
	chUpdateSvrState   chan struct{}                // Channel to notify the main loop of a server state change.
	writeLock          sync.Mutex                   // Mutex to protect writes to the shared memory.
	funcIDStr          string                       // String representation of the service's function ID.
	srcEntityIDStr     string                       // String representation of the service's entity ID.
	meshTags           []string                     // Tags for service discovery.
	handler            transport.DispatcherReceiver // The upper-layer handler for received messages.
	creator            transport.MsgCreator         // The factory for creating message objects.
	wg                 *sync.WaitGroup              // WaitGroup for managing goroutine shutdown.
	EncoderPool        *transport.EncoderPool       // Pool for reusing encoders to reduce allocations.
	pkgChan            chan sidecarMsg              // Channel for queuing messages to be sent over the Unix socket.
	cachedMsg          []*transport.TransRecvPkg    // Cache for messages received during certain state transitions.
	recvWG             *sync.WaitGroup              // WaitGroup specifically for the receiving goroutine.
	ctx                context.Context              // Main context for the transport.
	cancel             context.CancelFunc           // Cancel function for the main context.
	recvCtx            context.Context              // Context for the receiving goroutine.
	recvCancel         context.CancelFunc           // Cancel function for the receiving goroutine.

}

var _ transport.SSTransport = (*Sidecar)(nil)

// FactoryName returns the name of this transport factory.
func (t *Sidecar) FactoryName() string {
	return FactoryName
}

// Start initializes and starts the sidecar transport service.
func (t *Sidecar) Start(opt transport.TransportOption) error {
	if opt.Handler == nil || opt.Creator == nil {
		return fmt.Errorf("handler:%T or creator:%T is nil", opt.Handler, opt.Creator)
	}
	log.Info().Msg("Sidecar Start")
	t.handler = opt.Handler
	t.creator = opt.Creator

	t.wg.Add(1)
	t.recvWG.Add(1)

	t.recvCtx, t.recvCancel = context.WithCancel(t.ctx)
	go t.recv(t.recvCtx)

	log.Info().Msg("sidecar Start Succeed")
	return nil
}

// Stop gracefully stops the sidecar transport service.
func (t *Sidecar) Stop() error {
	defer func() {
		t.state = _stateStopped
	}()
	log.Info().Msg("sidecar Stop begin")
	if t.state == _stateStopped {
		return nil
	}

	// Cancel the main context to signal all goroutines to stop.
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}

	// Wait for all main goroutines to finish.
	t.wg.Wait()

	log.Info().Msg("sidecar Stop Succeed")
	return nil
}

// StopRecv stops only the receiving part of the sidecar transport.
func (t *Sidecar) StopRecv() error {
	log.Info().Msg("sidecar Stop Recv begin")
	if t.recvCancel != nil {
		t.recvCancel()
		t.recvCancel = nil
	}

	// Wait for the receiving goroutine to finish.
	t.recvWG.Wait()

	log.Info().Msg("sidecar Stop Recv Succeed")
	return nil
}

// OnFastExitStart is a hook called when the server initiates a fast exit.
func (t *Sidecar) OnFastExitStart() {
	t.SetSvrState(pb.ESvrState_ST_FASTEXIT)
	// A short sleep to allow the state change to propagate.
	log.Info().Msg("sidecar fastexit sleep begin")
	time.Sleep(time.Millisecond * 500) //nolint:mnd
	log.Info().Msg("sidecar fastexit sleep end")
}

// OnGracefulExitStart is a hook called when the server initiates a graceful exit.
func (t *Sidecar) OnGracefulExitStart() {
	t.SetSvrState(pb.ESvrState_ST_GRACEFUL_EXIT)
}

// SendToServer sends a message to another server via the sidecar.
func (t *Sidecar) SendToServer(pkg *message.TransSendPkg) error {
	if pkg.RouteHdr == nil || pkg.PkgHdr == nil {
		return fmt.Errorf("package headers are nil: RouteHdrNil:%t, HdrNil:%t", pkg.RouteHdr == nil, pkg.PkgHdr == nil)
	}

	// Populate necessary routing information.
	srcEntityID := runtime.GetEntityID()
	if srcEntityIDStr, ok := pkg.PkgHdr.GetMeta()["SrcEntityID"]; ok {
		if entityID, err := strconv.Atoi(srcEntityIDStr); err != nil {
			log.Error().Str("Meta.SrcEntityID", srcEntityIDStr).Err(err).Msg("SendToServer: invalid SrcEntityID in metadata")
		} else {
			srcEntityID = uint32(entityID)
			delete(pkg.PkgHdr.GetMeta(), "SrcEntityID")
		}
	}
	pkg.RouteHdr.SrcEntityID = srcEntityID
	pkg.RouteHdr.SrcSetVersion = runtime.GetSetVersion()
	pkg.RouteHdr.MsgID = pkg.PkgHdr.GetMsgID()

	// Legacy fields, may be needed for compatibility.
	pkg.PkgHdr.SvrPkgSeq = 1
	pkg.PkgHdr.CliPkgSeq = 1

	// Add pipe index to metadata for request/notification messages to support routing back to this instance.
	info, ok := message.GetProtoInfo(pkg.PkgHdr.GetMsgID())
	if !ok {
		return fmt.Errorf("failed to get proto info for msgid:%s", pkg.PkgHdr.GetMsgID())
	}
	if info.IsNtf() || info.IsReq() {
		shmpipeIdx := strconv.FormatUint(uint64(t.shmpipeIdx), 10)
		if pkg.PkgHdr.GetMeta() == nil {
			pkg.PkgHdr.Meta = map[string]string{t.shmpipeIdxMetaName: shmpipeIdx}
		} else {
			pkg.PkgHdr.Meta[t.shmpipeIdxMetaName] = shmpipeIdx
		}
	}

	buffer := t.EncoderPool.Get()
	defer t.EncoderPool.Put(buffer)
	if err := transport.EncodeSSMsgWithBuffer(pkg, buffer); err != nil {
		return fmt.Errorf("EncodeSSMsg failed: %w", err)
	}
	if err := t.mWriteShm(append(buffer.Datas[0], buffer.Datas[1]...), buffer.Datas[2:]...); err != nil {
		return fmt.Errorf("mWriteShm failed: %w", err)
	}
	transport.StatSendMsg(pkg, buffer.Length(), 0)
	return nil
}

// SetSvrState updates the server's state and notifies the sidecar.
func (t *Sidecar) SetSvrState(svrState pb.ESvrState) {
	oldSvrState := atomic.SwapInt32(&t.svrState, int32(svrState))
	log.Info().Int32("OldState", oldSvrState).Int32("NewState", int32(svrState)).Msg("SetSvcState")
	if oldSvrState != int32(svrState) {
		select {
		case t.chUpdateSvrState <- struct{}{}:
			log.Info().Int32("NewState", int32(svrState)).Msg("SetSvcState: Notified UnixSock of state change")
		default:
			log.Error().Msg("SetSvcState: Notify channel is full")
		}
	}
}

// ForwardMsg forwards a message, typically used during server migration between old and new instances.
func (t *Sidecar) ForwardMsg(pkg *transport.TransSendPkg) error {
	if pkg.RouteHdr == nil || pkg.PkgHdr == nil {
		return fmt.Errorf("package headers are nil: RouteHdrNil:%t, HdrNil:%t", pkg.RouteHdr == nil, pkg.PkgHdr == nil)
	}

	// Modify the route header to specify an internal route to the target pipe index.
	// This directs the message to the other server instance via the sidecar.
	pkg.RouteHdr.RouteType = &pb.RouteHead_Internal{
		Internal: &pb.InternalRoute{
			PipeIdx: int32(t.shmpipeIdx),
		},
	}

	buffer := t.EncoderPool.Get()
	defer t.EncoderPool.Put(buffer)
	if err := transport.EncodeSSMsgWithBuffer(pkg, buffer); err != nil {
		return fmt.Errorf("EncodeSSMsg failed: %w", err)
	}

	if err := t.mWriteShm(append(buffer.Datas[0], buffer.Datas[1]...), buffer.Datas[2:]...); err != nil {
		return fmt.Errorf("mWriteShm failed: %w", err)
	}
	return nil
}

// IsToCursvr checks if a received message is intended for the current server instance.
// This is crucial during migration to decide whether to process a message or forward it.
func (t *Sidecar) IsToCursvr(hdr *pb.PackageHead) bool {
	if hdr == nil {
		log.Error().Msg("IsToCursvr: Package head is nil")
		return false
	}

	// If the pipe index in the metadata matches the current instance's pipe index, it's for us.
	v, ok := hdr.GetMeta()[t.shmpipeIdxMetaName]
	if !ok {
		// If no pipe index is specified, assume it's for the current server.
		return true
	}
	return v == strconv.FormatUint(uint64(t.shmpipeIdx), 10)
}

// GetPipeIdx returns the shared memory pipe index used by this transport instance.
func (t *Sidecar) GetPipeIdx() uint8 {
	return uint8(t.shmpipeIdx)
}

// NtfChgRoute sends a notification to the sidecar to switch routes.
func (t *Sidecar) NtfChgRoute() error {
	regMsg := &pb.MsgReqUnixSockChgPipeIdx{
		Pipeidx: int32(t.shmpipeIdx),
	}

	m := sidecarMsg{
		id:  int(pb.EPBMsgType_PBMT_REQ_UNIXSOCK_CHG_PIPEIDX),
		msg: regMsg,
	}

	select {
	case t.pkgChan <- m:
	default:
		return fmt.Errorf("failed to send route change notification, pkgchan is full")
	}

	return nil
}

// CheckOldsvrStatus checks if the other pipe (belonging to the old server) is locked.
func (t *Sidecar) CheckOldsvrStatus() bool {
	if t.cfg.ShmpipeCnt <= 1 {
		return false
	}
	oldSvrPipeIdx := (t.shmpipeIdx + 1) % t.cfg.ShmpipeCnt
	fullPath := fmt.Sprintf("%s/%s%d", t.cfg.getShmDirPath(), t.cfg.getFileLockName(), oldSvrPipeIdx)
	return file.IsLock(fullPath)
}

// Unregister manually triggers the unregister process by closing the Unix socket.
func (t *Sidecar) Unregister() error {
	t.manualCloseUnixsocket()
	return nil
}

// run is the main loop for the sidecar transport, managing state and communication.
func (t *Sidecar) run(ctx context.Context, st *sync.WaitGroup) {
	log.Info().Msg("Sidecar run Goroutine begin")
	nextHBTime := time.Now().UnixNano()/1000000 + int64(t.cfg.getUnixHBTimeMS())
	defer func() {
		if t.wg != nil {
			t.wg.Done()
		}
		if st != nil {
			st.Done()
		}
		log.Info().Msg("Sidecar run Goroutine done")
	}()

	for {
		log.Info().Int("state", int(t.state)).Msg("Sidecar running")
		switch t.state {
		case _stateInited, _stateConnecting:
			// Attempt to register with the sidecar.
			if err := t.register(); err != nil {
				log.Error().Err(err).Msg("Sidecar register Failed")
			} else {
				log.Info().Msg("Sidecar register Succeed")
				t.state = _stateSucceed
				// Signal that initialization is complete.
				if st != nil {
					st.Done()
					st = nil
				}
			}
		case _stateSucceed:
			// In the connected state, perform regular updates.
			nextHBTime = t.updateUnixMsg(nextHBTime)
			t.usageStat()
		case _stateStopped:
			// If stopped, exit the loop.
			return
		}

		// Wait until the next update cycle.
		var result bool
		if nextHBTime, result = t.updateIdle(ctx, nextHBTime); !result {
			return
		}
	}
}

// updateUnixMsg handles communication over the Unix socket in the connected state.
func (t *Sidecar) updateUnixMsg(nextHBTime int64) int64 {
	nowMs := time.Now().UnixNano() / 1000000

	// 1. Send Heartbeat if it's time.
	if nowMs >= nextHBTime {
		hbMsg := pb.MsgNtyUnixSockHeartBeat{
			State:      pb.ESvrState(atomic.LoadInt32(&t.svrState)),
			PipeIdx:    int32(t.shmpipeIdx),
			SvrUnqueid: runtime.GetSvrBuildTime(),
			SetVersion: runtime.GetSetVersion(),
		}
		if err := t.writeUxPB(t.conn, int(pb.EPBMsgType_PBMT_NTY_UNIXSOCK_HEATBEAT), &hbMsg); err != nil {
			log.Error().Err(err).Msg("Sidecar SendHBMsgFailed, changing state to connecting")
			t.state = _stateConnecting
			return nextHBTime
		}
		nextHBTime = nowMs + int64(t.cfg.getUnixHBTimeMS())
	}

	// 2. Send any queued control messages.
	select {
	case m := <-t.pkgChan:
		if err := t.writeUxPB(t.conn, m.id, m.msg); err != nil {
			log.Error().Err(err).Msg("Sidecar send pkgchan msg Failed, changing state to connecting")
			t.state = _stateConnecting
		}
	default:
	}

	// 3. Try to read and handle a message from the Unix socket.
	if err := t.tryReadAndHandleUxSockMsg(); err != nil {
		log.Error().Int("pipeidx", t.shmpipeIdx).Err(err).Msg("tryReadAndHandleUxSockMsg failed, changing state to connecting")
		t.state = _stateConnecting
	}
	return nextHBTime
}

// updateIdle is the waiting part of the main loop, handling various events.
func (t *Sidecar) updateIdle(ctx context.Context, nextHBTime int64) (int64, bool) {
	var updateInternMS int
	if t.state == _stateSucceed {
		updateInternMS = _unixSockUpdateInternMS
	} else {
		// Use a longer retry interval if not connected.
		updateInternMS = t.cfg.UnixSockRetryMS
	}

	timer := time.NewTimer(time.Millisecond * time.Duration(updateInternMS))
	select {
	case <-ctx.Done():
		// Main context cancelled, start shutdown.
		log.Info().Msg("Sidecar run Goroutine: Context Done received")
		if err := t.deRegister(); err != nil {
			log.Error().Err(err).Msg("Sidecar Unix Goroutine DeRegister Failed")
		}
		log.Info().Msg("Sidecar run Goroutine deRegister Finished")
		t.state = _stateStopped
		return nextHBTime, false
	case <-t.closeUnixSocket:
		// Manual close triggered for migration.
		log.Info().Msg("Sidecar: manual close of unixsocket triggered, starting migration")
		if t.conn == nil {
			t.state = _stateStopped
			log.Error().Msg("conn is nil, changing state to stop")
			return 0, false
		}
		_ = t.conn.Close()
		t.state = _stateMigrate
		return nextHBTime, false
	case <-t.stopRecv:
		// Critical error in another goroutine (e.g., renewing shm lock).
		log.Fatal().Msg("Renew pipe shmlock fail, exiting")
		return nextHBTime, false
	case <-t.chUpdateSvrState:
		// Server state changed, force an immediate heartbeat.
		nextHBTime = time.Now().UnixNano() / 1000000
	case <-timer.C:
		// Timer fired, continue the loop.
	}
	return nextHBTime, true
}

// recv is the main loop for the receiving goroutine. It reads from shared memory and dispatches messages.
func (t *Sidecar) recv(ctx context.Context) {
	log.Info().Msg("Sidecar Recv Goroutine begin")
	ticker := time.NewTicker(time.Second * 1)
	defer func() {
		ticker.Stop()
		if t.wg != nil {
			t.wg.Done()
		}
		if t.recvWG != nil {
			t.recvWG.Done()
		}
	}()

	idle := 0
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Sidecar Recv Goroutine: Context Done received")
			return
		case <-t.stopRecv:
			log.Fatal().Msg("Renew pipe shmlock fail, exiting")
			return
		case <-ticker.C:
			t.delayStat() // Periodically report delay stats.
		default:
		}

		// Sleep if idle to reduce CPU usage.
		idle++
		if idle >= _recvIdleCntToSleep {
			time.Sleep(_recvSleepTime)
		}
		if t.handler == nil {
			continue
		}

		// Dispatch any cached messages first.
		t.tryDispatchAllCachedMsg()

		// Read a message from shared memory.
		data := t.readShm()
		if data == nil {
			continue
		}

		// Reset idle count since we received a message.
		idle = 0
		for {
			// Decode and dispatch the message.
			pkg, err := transport.DecodeSSMsgWithoutBody(data, t.creator)
			if err != nil {
				log.Error().Err(err).Msg("Sidecar recv: DecodeSSMsg error")
				break
			}
			transport.StatRecvMsg(pkg, len(data))

			if err := t.handler.OnRecvTransportPkg(&transport.TransportDelivery{
				TransSendBack: t.SendToServer,
				Pkg:           pkg,
			}); err != nil {
				log.Error().Str("msgid", pkg.PkgHdr.GetMsgID()).Int("pip", t.shmpipeIdx).
					Err(err).Msg("Sidecar recv: dispatch error")
			}
			break
		}
	}
}

// register attempts to connect and register with the sidecar via the Unix socket.
func (t *Sidecar) register() error { //nolint:funlen,revive
	if t.state != _stateConnecting && t.state != _stateInited {
		return fmt.Errorf("current state is %v, expected 'connecting' or 'inited'", t.state)
	}
	if t.conn != nil {
		if err := t.conn.Close(); err != nil {
			return fmt.Errorf("failed to close existing connection: %w", err)
		}
		t.conn = nil
	}

	// Dial the Unix socket.
	addr, err := net.ResolveUnixAddr("unix", t.cfg.getUnixSockPath())
	if err != nil {
		return fmt.Errorf("failed to resolve unix address %s: %w", t.cfg.getUnixSockPath(), err)
	}
	conn, err := net.DialUnix("unix", nil, addr)
	if err != nil {
		return fmt.Errorf("failed to dial unix socket: %w", err)
	}

	defer func() {
		// If registration is not successful, close the connection.
		if t.state != _stateSucceed {
			if err1 := conn.Close(); err1 != nil {
				log.Error().Err(err1).Msg("Defer close conn err")
			}
		}
	}()

	// Create and send the registration request.
	regMsg := &pb.MsgReqUnixSockRegister{
		SvcInfo: &pb.ServiceQueryInfo{
			ServiceName: t.funcIDStr,
			ServiceID:   t.srcEntityIDStr,
			Tags:        t.meshTags,
			SetVersion:  runtime.GetSetVersion(),
		},
		RegisterOption: &pb.NamingRegiserOption{
			IsStatefulService:   false,
			NeedMasterSelection: false,
			NeedBackup:          false,
			StaticWeight:        1,
			DynamicWeight:       1,
			IsHealth:            true,
			PipeIdx:             int32(t.shmpipeIdx),
			SvrUnqueid:          runtime.GetSvrBuildTime(),
			Version:             t.version,
		},
	}
	if err = t.writeUxPB(conn, int(pb.EPBMsgType_PBMT_REQ_UNIXSOCK_REGISTER), regMsg); err != nil {
		return fmt.Errorf("failed to send MsgReqUnixSockRegister: %w", err)
	}

	// Wait for and read the registration response.
	var resp proto.Message
	_, resp, err = t.readUxPB(conn, int(pb.EPBMsgType_PBMT_RSP_UNIXSOCK_REGISTER), _unixSockReadTimeoutSec*1000)
	if err != nil {
		return fmt.Errorf("failed to receive MsgResUnixSockRegister: %w", err)
	}

	respMsg, ok := resp.(*pb.MsgResUnixSockRegister)
	if !ok {
		return fmt.Errorf("failed to convert response to MsgResUnixSockRegister, got type %s", reflect.TypeOf(resp).Name())
	}
	if respMsg.GetResult() == nil || respMsg.GetResult().GetErrCode() != 0 {
		if respMsg.GetResult() != nil {
			return fmt.Errorf("registration failed with error code %d: %s", respMsg.GetResult().GetErrCode(), respMsg.GetResult().GetReason())
		}
		return fmt.Errorf("registration response result is nil")
	}

	// Success! Update state.
	t.conn = conn
	t.state = _stateSucceed
	t.version = respMsg.GetVersion()
	t.main = respMsg.GetIsMain() == 1

	return nil
}

// deRegister sends an unregister notification to the sidecar.
func (t *Sidecar) deRegister() error {
	if t.conn == nil {
		return fmt.Errorf("deRegister connection is nil")
	}

	ntyMsg := &pb.MsgNtyUnixSockUnRegister{
		Dummy:      1,
		ServiceId:  t.srcEntityIDStr,
		PipeIdx:    int32(t.shmpipeIdx),
		SvrUnqueid: runtime.GetSvrBuildTime(),
	}

	if err := t.writeUxPB(t.conn, int(pb.EPBMsgType_PBMT_NTY_UNIXSOCK_UNREGISTER), ntyMsg); err != nil {
		return fmt.Errorf("failed to send MsgNtyUnixSockUnRegister: %w", err)
	}

	// Drain any remaining messages from the socket.
	for {
		_, _, err := t.readUxPB(t.conn, 0, _unixSockUpdateInternMS)
		if err != nil {
			var e net.Error
			if errors.As(err, &e) && e.Timeout() {
				// Normal timeout, no more messages.
				continue
			}
			// Connection closed or other error.
			break
		}
	}

	return nil
}

// tryReadAndHandleUxSockMsg attempts to read a message from the Unix socket and handle it.
func (t *Sidecar) tryReadAndHandleUxSockMsg() error {
	// Use a short timeout to make it non-blocking.
	msgID, uxMsg, err := t.readUxPB(t.conn, 0, _unixSockUpdateInternMS)
	if err != nil {
		var e net.Error
		if errors.As(err, &e) && e.Timeout() {
			// This is expected when there are no incoming messages.
			return nil
		}
		return err
	}

	switch msgID {
	case int(pb.EPBMsgType_PBMT_NTY_UNIXSOCK_HEATBEAT):
		// Received a heartbeat from the sidecar.
		if _, ok := uxMsg.(*pb.MsgNtyUnixSockHeartBeat); !ok {
			return fmt.Errorf("failed to convert message to MsgNtyUnixSockHeartBeat, got type %s", string(uxMsg.ProtoReflect().Descriptor().Name()))
		}
	case int(pb.EPBMsgType_PBMT_RES_UNIXSOCK_CHG_PIPEIDX):
		// Received a response to a pipe index change request.
		if _, ok := uxMsg.(*pb.MsgResUnixSockChgPipeIdx); !ok {
			return fmt.Errorf("failed to convert message to MsgResUnixSockChgPipeIdx, got type %s", string(uxMsg.ProtoReflect().Descriptor().Name()))
		}
		t.onRecvChgPipeidxRes()
	default:
		log.Info().Int("msgid", msgID).Msg("Received unexpected message ID from sidecar")
	}
	return nil
}

// hasData checks if there is data to be read from the shared memory.
func (t *Sidecar) hasData() bool {
	if t.state == _stateStopped || t.state == _stateNone || !t.shmChannel.readQueue.hasData() {
		return false
	}
	return true
}

// readShm reads a single message from the shared memory queue.
func (t *Sidecar) readShm() []byte {
	if t.state == _stateStopped || t.state == _stateNone || !t.shmChannel.readQueue.hasData() {
		return nil
	}
	data, err := t.shmChannel.readQueue.read(_shmMaxValidMsgSec)
	if err != nil {
		log.Error().Err(err).Msg("Sidecar ReadShm Error")
		return nil
	}
	return data
}

// writeShm writes a single data buffer to the shared memory write queue.
func (t *Sidecar) writeShm(data []byte, headLen int64) error {
	if t.state == _stateNone {
		return fmt.Errorf("Sidecar not inited when writeshm")
	}
	if len(data) == 0 || headLen > int64(len(data)) {
		return fmt.Errorf("invalid length: datalen:%d, headlen:%d", len(data), headLen)
	}

	t.writeLock.Lock()
	defer t.writeLock.Unlock()
	return t.shmChannel.writeQueue.write(data, uint32(headLen), uint32(len(data))-uint32(headLen))
}

// mWriteShm writes multiple data buffers to the shared memory write queue.
func (t *Sidecar) mWriteShm(routeHdr []byte, data ...[]byte) error {
	if t.state == _stateNone {
		return fmt.Errorf("Sidecar not inited when mWriteShm")
	}
	if len(data) == 0 || len(routeHdr) == 0 {
		return fmt.Errorf("invalid data length: cnt:%d, Routeheadlen:%d", len(data), len(routeHdr))
	}

	t.writeLock.Lock()
	defer t.writeLock.Unlock()
	return t.shmChannel.writeQueue.mWrite(routeHdr, data...)
}

// writeUxPB marshals and writes a protobuf message to the Unix socket.
func (t *Sidecar) writeUxPB(conn *net.UnixConn, msgID int, m proto.Message) error {
	if conn == nil || m == nil {
		return fmt.Errorf("connection or message is nil")
	}
	buff := make([]byte, _unixSockHdrSize, _unixSockMsgBuffSize)
	var head unixHead

	sendData, err := proto.Marshal(m)
	if err != nil {
		return fmt.Errorf("failed to marshal protobuf message: %w", err)
	}
	head.msgID = int32(msgID)
	head.headSize = int32(len(sendData))
	head.msgSize = head.headSize + _unixSockHdrSize
	head.Encode(buff)
	buff = append(buff, sendData...)

	if err = conn.SetWriteDeadline(time.Now().Add(_unixSockWriteTimeoutSec * time.Second)); err != nil {
		return fmt.Errorf("failed to set write deadline: %w", err)
	}
	if err2 := writeFull(conn, buff); err2 != nil {
		return fmt.Errorf("failed to write full message: %w", err2)
	}
	return nil
}

// readUxPB reads and unmarshals a protobuf message from the Unix socket.
func (t *Sidecar) readUxPB(conn *net.UnixConn, expectMsgID int, timeoutMS int64) (int, proto.Message, error) {
	if conn == nil {
		return 0, nil, fmt.Errorf("connection is nil")
	}
	if timeoutMS >= 0 {
		if err := conn.SetReadDeadline(time.Now().Add(time.Millisecond * time.Duration(timeoutMS))); err != nil {
			return 0, nil, fmt.Errorf("failed to set read deadline: %w", err)
		}
	}

	// Read header
	unixHdr := make([]byte, _unixSockHdrSize)
	if err := readFull(conn, unixHdr); err != nil {
		return 0, nil, fmt.Errorf("failed to read full header: %w", err)
	}

	var head unixHead
	if err := head.Decode(unixHdr); err != nil {
		return 0, nil, fmt.Errorf("failed to decode unix header: %w", err)
	}
	if expectMsgID > 0 && int(head.msgID) != expectMsgID {
		return 0, nil, fmt.Errorf("unexpected message ID: got %d, expected %d", head.msgID, expectMsgID)
	}

	// Read body
	if err := conn.SetReadDeadline(time.Now().Add(time.Millisecond * time.Duration(timeoutMS))); err != nil {
		return 0, nil, fmt.Errorf("failed to set read deadline for body: %w", err)
	}
	msgBuf := make([]byte, head.msgSize-_unixSockHdrSize)
	if err := readFull(conn, msgBuf); err != nil {
		return 0, nil, fmt.Errorf("failed to read full body: %w", err)
	}

	// Unmarshal to the correct protobuf type.
	var m proto.Message
	switch head.msgID {
	case int32(pb.EPBMsgType_PBMT_RSP_UNIXSOCK_REGISTER):
		m = &pb.MsgResUnixSockRegister{}
	case int32(pb.EPBMsgType_PBMT_NTY_UNIXSOCK_HEATBEAT):
		m = &pb.MsgNtyUnixSockHeartBeat{}
	case int32(pb.EPBMsgType_PBMT_RES_UNIXSOCK_CHG_PIPEIDX):
		m = &pb.MsgResUnixSockChgPipeIdx{}
	default:
		return 0, nil, fmt.Errorf("unknown message ID: %d", head.msgID)
	}

	if err := proto.Unmarshal(msgBuf, m); err != nil {
		return 0, nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}
	return int(head.msgID), m, nil
}

// initMeshAddr initializes service discovery and mesh-related identifiers.
func (t *Sidecar) initMeshAddr() {
	t.funcIDStr = strconv.Itoa(runtime.GetFuncID())
	t.srcEntityIDStr = runtime.GetEntityIDStr()
	t.meshTags = []string{fmt.Sprintf("area%d", runtime.GetAreaID())}
	t.shmpipeIdxMetaName = fmt.Sprintf("%s%s", runtime.GetEntityIDStr(), _baseShmpipeIdxMetaName)
}

// usageStat collects and reports metrics on shared memory queue usage.
func (t *Sidecar) usageStat() {
	if t.state != _stateSucceed && t.state != _stateMigrate {
		return
	}

	const bp = 100 // for percentage calculation
	var readPercent, writePercent metrics.Value
	if rs := t.shmChannel.readQueue.queueBuffSize(); rs != 0 {
		maxUnRead := atomic.SwapUint32(&t.shmChannel.readQueue.shmStat.maxUsedShmSize, 0)
		readPercent = metrics.Value(maxUnRead) / metrics.Value(rs) * bp
		metrics.UpdateMaxGaugeWithDimGroup(metrics.NameSidecarReadMemUsageMaxPercent,
			metrics.GroupStrix, readPercent, metrics.Dimension{
				metrics.DimPipeIdx: strconv.Itoa(int(t.GetPipeIdx())),
			})
	}
	if ws := t.shmChannel.writeQueue.queueBuffSize(); ws != 0 {
		maxUnWrite := atomic.SwapUint32(&t.shmChannel.writeQueue.shmStat.maxUsedShmSize, 0)
		writePercent = metrics.Value(maxUnWrite) / metrics.Value(ws) * bp
		metrics.UpdateMaxGaugeWithDimGroup(metrics.NameSidecarWriteMemUsageMaxPercent,
			metrics.GroupStrix, writePercent, metrics.Dimension{
				metrics.DimPipeIdx: strconv.Itoa(int(t.GetPipeIdx())),
			})
	}
}

// delayStat collects and reports metrics on shared memory read delays.
func (t *Sidecar) delayStat() {
	if t.state != _stateSucceed && t.state != _stateMigrate {
		return
	}

	metrics.UpdateMaxGaugeWithDimGroup(metrics.NameSidecarReadDelayMaxMS,
		metrics.GroupStrix, metrics.Value(t.shmChannel.readQueue.shmStat.readDelayMaxMS), metrics.Dimension{
			metrics.DimPipeIdx: strconv.Itoa(int(t.GetPipeIdx())),
		})
	atomic.StoreUint64(&t.shmChannel.readQueue.shmStat.readDelayMaxMS, 0)

	var readDelayAvgMS metrics.Value
	readDelayCnt := atomic.LoadUint64(&t.shmChannel.readQueue.shmStat.readDelayCnt)
	if readDelayCnt != 0 {
		readDelayTotalMS := atomic.LoadUint64(&t.shmChannel.readQueue.shmStat.readDelayTotalMS)
		readDelayAvgMS = metrics.Value(readDelayTotalMS) / metrics.Value(readDelayCnt)
	}
	atomic.StoreUint64(&t.shmChannel.readQueue.shmStat.readDelayCnt, 0)
	atomic.StoreUint64(&t.shmChannel.readQueue.shmStat.readDelayTotalMS, 0)
	metrics.UpdateMaxGaugeWithDimGroup(metrics.NameSidecarReadDelayAvgMS,
		metrics.GroupStrix, readDelayAvgMS, metrics.Dimension{
			metrics.DimPipeIdx: strconv.Itoa(int(t.GetPipeIdx())),
		})
}

// manualCloseUnixsocket sends a signal to the main loop to close the Unix socket.
func (t *Sidecar) manualCloseUnixsocket() {
	select {
	case t.closeUnixSocket <- struct{}{}:
		log.Info().Msg("Manual close unixsocket signal sent")
	default:
		log.Error().Msg("closeUnixSocket channel is full")
	}
}

// onRecvChgPipeidxRes is the handler for a pipe index change response.
func (t *Sidecar) onRecvChgPipeidxRes() {
	// Placeholder for future logic.
}

// cacheMsg adds a received package to a temporary cache.
func (t *Sidecar) cacheMsg(pkg *transport.TransRecvPkg) {
	if t.cachedMsg == nil {
		t.cachedMsg = make([]*transport.TransRecvPkg, 0, 1000) //nolint:gomnd
	}
	t.cachedMsg = append(t.cachedMsg, pkg)
}

// tryDispatchAllCachedMsg dispatches all messages currently in the cache.
func (t *Sidecar) tryDispatchAllCachedMsg() {
	if len(t.cachedMsg) == 0 {
		return
	}

	handleStartTime := time.Now().UnixMilli()
	for i := 0; i < len(t.cachedMsg); i++ {
		if err := t.handler.OnRecvTransportPkg(&transport.TransportDelivery{
			TransSendBack: t.SendToServer,
			Pkg:           t.cachedMsg[i],
		}); err != nil {
			log.Error().Str("msgid", t.cachedMsg[i].PkgHdr.GetMsgID()).Err(err).
				Msg("Failed to dispatch cached message")
		}
	}

	log.Info().Int("count", len(t.cachedMsg)).Int64("cost_ms", time.Now().UnixMilli()-handleStartTime).
		Msg("Dispatched cached messages")
	t.cachedMsg = nil
}
