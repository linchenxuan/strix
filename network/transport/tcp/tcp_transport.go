// Package tcp provides the interfaces and concrete implementations for network communication.
// This file implements a TCP-based transport layer.
package tcp

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/linchenxuan/strix/log"
	"github.com/linchenxuan/strix/metrics"
	"github.com/linchenxuan/strix/network/transport"
)

// TCPTransportCfg holds all configuration parameters for the TCPTransport.
type TCPTransportCfg struct {
	Tag             string `mapstructure:"tag"`             // A unique identifier for this transport instance.
	IdleTimeout     uint32 `mapstructure:"idleTimeout"`     // The maximum duration in seconds a connection can be idle before being closed.
	Crypt           uint32 `mapstructure:"crypt"`           // Specifies the encryption method to be used (0 for none).
	Addr            string `mapstructure:"addr"`            // The network address (e.g., "host:port") for the server to listen on.
	ConnType        string `mapstructure:"connType"`        // The type of connection, e.g., "server" or "client".
	FrameMetaKey    string `mapstructure:"frameMetaKey"`    // A key used for frame metadata, potentially for custom framing logic.
	SendChannelSize uint32 `mapstructure:"sendChannelSize"` // The buffer size of the send channel for each connection's write loop.
	MaxBufferSize   int    `mapstructure:"maxBufferSize"`   // The maximum size of the read/write buffers for the underlying TCP connection.
}

// GetName returns the configuration key for TCPTransportCfg.
func (c *TCPTransportCfg) GetName() string {
	return "tcp_transport"
}

// Validate checks if the TCPTransportCfg parameters are valid.
func (c *TCPTransportCfg) Validate() error {
	if c.Addr == "" {
		return errors.New("Addr cannot be empty")
	}
	if c.MaxBufferSize <= 0 {
		return errors.New("MaxBufferSize must be positive")
	}
	if c.SendChannelSize <= 0 {
		return errors.New("SendChannelSize must be positive")
	}
	if c.IdleTimeout <= 0 {
		return errors.New("IdleTimeout must be positive")
	}
	return nil
}

// TCPTransport implements the Transport interface for TCP. It acts as a server,
// listening for incoming connections and managing a pool of active connection contexts.
type TCPTransport struct {
	*TCPTransportCfg                              // Embedded configuration.
	uidToConn        map[uint64]*tcpctx           // A map from a connection's unique ID to its context.
	lock             sync.RWMutex                 // Protects concurrent access to uidToConn.
	receiver         transport.DispatcherReceiver // The next layer in the stack (typically the dispatcher) to handle incoming messages.
	creator          transport.MsgCreator         // A factory for creating message objects from IDs.
	cancel           context.CancelFunc           // A function to cancel the transport's main context, shutting it down.
}

// NewTCPTransport creates a new TCPTransport instance with the given configuration.
func NewTCPTransport(cfg *TCPTransportCfg) (*TCPTransport, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid TCPTransportCfg: %w", err)
	}
	return &TCPTransport{
		TCPTransportCfg: cfg,
		uidToConn:       make(map[uint64]*tcpctx),
		lock:            sync.RWMutex{},
	}, nil
}

// GetConfigName returns the configuration key for use with a config manager.
func (t *TCPTransport) GetConfigName() string {
	return "tcp_transport"
}

// Start brings the TCP transport online. It begins listening for incoming TCP connections
// on the configured address and launches a goroutine to accept them.
func (t *TCPTransport) Start(opt transport.TransportOption) error {
	metrics.IncrCounterWithGroup("net", "transport_start_total", 1)
	t.receiver = opt.Handler
	t.creator = opt.Creator

	if t.TCPTransportCfg == nil {
		return errors.New("TCPTransportCfg is nil")
	}
	if t.Addr == "" {
		return errors.New("Addr is empty")
	}

	tcpAddr, err := net.ResolveTCPAddr("tcp", t.Addr)
	if err != nil {
		return fmt.Errorf("failed to resolve TCP address '%s': %w", t.Addr, err)
	}

	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on TCP address '%s': %w", t.Addr, err)
	}

	metrics.IncrCounterWithDimGroup("net", "transport_start_success_total", 1, map[string]string{"transport_type": "tcp"})

	var ctx context.Context
	ctx, t.cancel = context.WithCancel(context.Background())
	go t.serve(ctx, listener)
	log.Info().Str("address", t.Addr).Msg("TCP transport started and listening")
	return nil
}

// Stop gracefully shuts down the TCP transport by canceling its main context,
// which in turn closes the listener and all active connections.
func (t *TCPTransport) Stop() error {
	if t.cancel != nil {
		t.cancel()
	}
	return nil
}

// StopRecv is not supported by this TCP transport implementation.
func (t *TCPTransport) StopRecv() error {
	return errors.New("tcp transport does not support stopping receive independently")
}

// OnGracefulExitStart is a hook for graceful shutdown procedures.
func (t *TCPTransport) OnGracefulExitStart() {}

// serve is the main accept loop for the server. It waits for new connections,
// and for each one, it creates a `tcpctx` and launches it in a new goroutine.
func (t *TCPTransport) serve(ctx context.Context, listener *net.TCPListener) {
	defer func() { _ = listener.Close() }()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("TCP transport context cancelled, stopping accept loop.")
			return
		default:
		}

		_ = listener.SetDeadline(time.Now().Add(time.Second)) // Set a deadline to allow checking the context.
		conn, err := listener.AcceptTCP()
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
				continue // This is expected, loop again.
			}
			log.Error().Err(err).Msg("Failed to accept TCP connection")
			return // A non-timeout error is likely fatal for the listener.
		}

		if err = conn.SetReadBuffer(t.MaxBufferSize); err != nil {
			log.Error().Err(err).Msg("Failed to set read buffer size")
			_ = conn.Close()
			continue
		}
		if err = conn.SetWriteBuffer(t.MaxBufferSize); err != nil {
			log.Error().Err(err).Msg("Failed to set write buffer size")
			_ = conn.Close()
			continue
		}

		// Create a new context to manage the lifecycle of this specific connection.
		cancelCtx, cancel := context.WithCancel(ctx)
		tctx := &tcpctx{
			ctx:        ctx,
			cancelCtx:  cancelCtx,
			cancel:     cancel,
			conn:       conn,
			localAddr:  conn.LocalAddr(),
			remoteAddr: conn.RemoteAddr(),
			sendCh:     make(chan *transport.TransSendPkg, t.SendChannelSize),
			transport:  t,
		}
		tctx.serve() // Launch the connection's read/write goroutines.
	}
}

// SendToClient sends a package to a specific client identified by its UID.
// It looks up the active connection for the UID and queues the package for sending.
func (t *TCPTransport) SendToClient(pkg *transport.TransSendPkg) error {
	t.lock.RLock()
	uid := pkg.GetPkgHdr().GetDstActorID()
	tctx, ok := t.uidToConn[uid]
	t.lock.RUnlock()

	if !ok {
		return fmt.Errorf("SendToClient failed: connection for UID %d not found", uid)
	}

	return tctx.SendToClient(pkg)
}

// SendToServer sends a package to a server. In this client-facing transport,
// it is functionally equivalent to SendToClient.
func (t *TCPTransport) SendToServer(pkg *transport.TransSendPkg) error {
	return t.SendToClient(pkg)
}

// CloseConn explicitly closes the connection for a given UID.
func (t *TCPTransport) CloseConn(uid uint64) error {
	t.lock.RLock()
	tctx, ok := t.uidToConn[uid]
	t.lock.RUnlock()
	if !ok {
		return fmt.Errorf("CloseConn failed: connection for UID %d not found", uid)
	}
	tctx.close()
	return nil
}

// removeConn removes a connection's context from the active connections map.
func (t *TCPTransport) removeConn(uid uint64) {
	t.lock.Lock()
	defer t.lock.Unlock()
	delete(t.uidToConn, uid)
}

// addConn adds a new connection's context to the active connections map.
func (t *TCPTransport) addConn(uid uint64, tcx *tcpctx) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.uidToConn[uid] = tcx
}

// getCurrentConnCount returns the number of currently active connections.
func (t *TCPTransport) getCurrentConnCount() int {
	t.lock.RLock()
	defer t.lock.RUnlock()
	return len(t.uidToConn)
}

// tcpctx represents the context and state for a single, active TCP connection.
// It manages the connection's lifecycle and runs two dedicated goroutines: one for
// reading and one for writing, to handle concurrent I/O efficiently.
type tcpctx struct {
	uid           uint64             // Unique ID of the client, read from the initial handshake.
	actorMetaKey  uint64             // Associated metadata key.
	secureKey     uint64             // Associated security key.
	ctx           context.Context    // The parent context from the transport.
	cancelCtx     context.Context    // The context for this specific connection.
	cancel        context.CancelFunc // The function to cancel `cancelCtx` and shut down this connection.
	lastReadTime  time.Time          // Timestamp of the last successful read, for idle timeouts.
	lastWriteTime time.Time          // Timestamp of the last successful write.
	conn          net.Conn           // The underlying network connection.
	localAddr     net.Addr
	remoteAddr    net.Addr
	closeOnce     sync.Once                    // Ensures the close logic is executed only once.
	sendCh        chan *transport.TransSendPkg // A buffered channel acting as the send queue for this connection.
	transport     *TCPTransport                // A reference back to the parent transport.
}

// close gracefully shuts down the connection. It is safe to call multiple times.
func (t *tcpctx) close() {
	t.closeOnce.Do(func() {
		log.Info().Uint64("uid", t.uid).Str("remote", t.remoteAddr.String()).Msg("Closing TCP connection")
		t.transport.removeConn(t.uid)
		metrics.IncrCounterWithGroup("net", "connection_close_total", 1)
		metrics.UpdateGaugeWithGroup("net", "current_connections", metrics.Value(t.transport.getCurrentConnCount()))
		t.cancel()         // Signal goroutines to stop.
		_ = t.conn.Close() // Close the underlying socket.
	})
}

// serve launches the dedicated read and write goroutines for this connection.
func (t *tcpctx) serve() {
	go t.serveSend()
	go t.serveRecv()
}

const (
	_uidLen       = 8 // Length of the UID in bytes.
	_metaKeyLen   = 8 // Length of the metadata key in bytes.
	_secureKeyLen = 8 // Length of the secure key in bytes.
)

// readUID performs the initial handshake by reading the client's UID, meta key, and secure key.
func (t *tcpctx) readUID() bool {
	_ = t.setReadDeadline()
	handshakeBuf := make([]byte, _uidLen+_metaKeyLen+_secureKeyLen)
	n, err := t.conn.Read(handshakeBuf)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read initial handshake")
		return false
	}
	if n != len(handshakeBuf) {
		log.Error().Int("read", n).Int("expected", len(handshakeBuf)).Msg("Mismatched length on handshake read")
		return false
	}

	t.uid = binary.LittleEndian.Uint64(handshakeBuf[0:8])
	t.actorMetaKey = binary.LittleEndian.Uint64(handshakeBuf[8:16])
	t.secureKey = binary.LittleEndian.Uint64(handshakeBuf[16:24])

	// Close any existing connection with the same UID to enforce a single session.
	_ = t.transport.CloseConn(t.uid)
	t.transport.addConn(t.uid, t)
	log.Info().Uint64("uid", t.uid).Str("remote", t.remoteAddr.String()).Msg("Successful handshake")
	return true
}

// readPreHead reads the fixed-size length prefix (PreHead) from the stream.
func (t *tcpctx) readPreHead() (*transport.PreHead, bool) {
	preHeadBuf := make([]byte, transport.PRE_HEAD_SIZE)
	n, err := t.conn.Read(preHeadBuf)
	if err != nil {
		// Do not log EOF or timeout errors as errors, as they are expected on close/idle.
		if !errors.Is(err, net.ErrClosed) && !errors.Is(err, context.DeadlineExceeded) {
			log.Error().Err(err).Msg("Failed to read PreHead")
		}
		return nil, false
	}
	if n != transport.PRE_HEAD_SIZE {
		log.Error().Int("read", n).Int("expected", transport.PRE_HEAD_SIZE).Msg("Mismatched length on PreHead read")
		return nil, false
	}

	preHead, err := transport.DecodePreHead(preHeadBuf)
	if err != nil {
		log.Error().Err(err).Msg("Failed to decode PreHead")
		return nil, false
	}

	if int(preHead.HdrSize+preHead.BodySize) > t.transport.MaxBufferSize {
		log.Error().Uint32("hdr", preHead.HdrSize).Uint32("body", preHead.BodySize).
			Int("max", t.transport.MaxBufferSize).Msg("Package size exceeds MaxBufferSize")
		return nil, false
	}
	return preHead, true
}

// beginServeRecv performs the initial setup for the receive loop, starting with the handshake.
func (t *tcpctx) beginServeRecv() error {
	if !t.readUID() {
		metrics.IncrCounterWithGroup("net", "connection_auth_failure_total", 1)
		return errors.New("readUID failed during handshake")
	}
	metrics.IncrCounterWithGroup("net", "connection_success_total", 1)
	metrics.UpdateGaugeWithGroup("net", "current_connections", metrics.Value(t.transport.getCurrentConnCount()))
	return nil
}

// recvPkg reads a single, complete message from the wire, decodes it, and dispatches it.
func (t *tcpctx) recvPkg(buf *bytes.Buffer) (quitLoop bool, err error) {
	// 1. Read the length prefix.
	_ = t.setReadDeadline()
	preHead, ok := t.readPreHead()
	if !ok {
		return true, errors.New("failed to read pre-header")
	}

	// 2. Read the full payload based on the length prefix.
	payloadLen := preHead.HdrSize + preHead.BodySize
	buf.Reset()
	buf.Grow(int(payloadLen))
	payloadBuf := buf.Bytes()[:payloadLen]
	n, err := t.conn.Read(payloadBuf)
	if err != nil {
		return true, fmt.Errorf("failed to read payload: %w", err)
	}
	if n != int(payloadLen) {
		return true, fmt.Errorf("payload length mismatch: expected %d, got %d", payloadLen, n)
	}

	// 3. Decode the payload into a structured package.
	pkg, err := transport.DecodeCSPkg(preHead, payloadBuf, t.transport.creator)
	if err != nil {
		log.Error().Err(err).Msg("Failed to decode CSPkg")
		return false, err // Non-fatal error, continue the loop.
	}
	if pkg.PkgHdr == nil {
		return false, errors.New("decoded package has nil header")
	}

	// 4. Dispatch the package to the next layer.
	delivery := &transport.TransportDelivery{
		TransSendBack: t.SendToClient,
		Pkg:           pkg,
	}

	if err = t.transport.receiver.OnRecvTransportPkg(delivery); err != nil {
		log.Error().Err(err).Str("msgId", pkg.PkgHdr.GetMsgID()).Msg("Error dispatching package")
		// The error is logged, but the connection is not necessarily terminated.
	}

	return false, nil
}

// serveRecv is the main loop for reading data from the connection.
func (t *tcpctx) serveRecv() {
	defer t.close()
	if err := t.beginServeRecv(); err != nil {
		log.Error().Err(err).Msg("Failed to start serve receive loop")
		return
	}

	buf := &bytes.Buffer{}
	for {
		select {
		case <-t.ctx.Done(): // Parent transport is shutting down.
			return
		case <-t.cancelCtx.Done(): // This specific connection is shutting down.
			return
		default:
			quit, err := t.recvPkg(buf)
			if err != nil {
				log.Debug().Err(err).Uint64("uid", t.uid).Msg("Error receiving package")
			}
			if quit {
				return // A fatal error occurred, exit the loop.
			}
		}
	}
}

// serveSend is the main loop for writing data to the connection. It reads packages
// from the `sendCh` and writes them to the socket, serializing all writes.
func (t *tcpctx) serveSend() {
	defer t.close()
	for {
		select {
		case <-t.ctx.Done():
			return
		case <-t.cancelCtx.Done():
			return
		case pkg := <-t.sendCh:
			if err := t.send(pkg); err != nil {
				log.Error().Err(err).Uint64("uid", t.uid).Msg("Failed to send package")
				return // A write error is typically fatal.
			}
		}
	}
}

// send serializes and writes a single package to the connection.
func (t *tcpctx) send(pkg *transport.TransSendPkg) error {
	buf := &bytes.Buffer{}
	preHeadBuf, err := transport.PackCSPkg(pkg, buf)
	if err != nil {
		return fmt.Errorf("failed to pack CSPkg: %w", err)
	}

	_ = t.setWriteDeadline()
	// Write PreHead and the rest of the payload in two calls.
	if _, err := t.conn.Write(preHeadBuf); err != nil {
		return fmt.Errorf("failed to write pre-head: %w", err)
	}
	if _, err := t.conn.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("failed to write payload: %w", err)
	}
	return nil
}

// setReadDeadline updates the read deadline for the connection to enforce idle timeouts.
func (t *tcpctx) setReadDeadline() error {
	if t.transport.IdleTimeout > 0 {
		return t.conn.SetReadDeadline(time.Now().Add(time.Duration(t.transport.IdleTimeout) * time.Second))
	}
	return nil
}

// setWriteDeadline updates the write deadline for the connection.
func (t *tcpctx) setWriteDeadline() error {
	if t.transport.IdleTimeout > 0 {
		return t.conn.SetWriteDeadline(time.Now().Add(time.Duration(t.transport.IdleTimeout) * time.Second))
	}
	return nil
}

// SendToClient queues a package to be sent to this connection's client.
// It is the entry point for application code to send data.
func (t *tcpctx) SendToClient(pkg *transport.TransSendPkg) error {
	if pkg.PkgHdr == nil {
		return errors.New("cannot send package with nil PkgHdr")
	}

	select {
	case t.sendCh <- pkg:
		return nil
	default:
		// The send channel is full, indicating back-pressure.
		log.Warn().Uint64("uid", t.uid).Msg("Send channel is full, dropping package")
		metrics.IncrCounterWithGroup("net", "send_channel_full_total", 1)
		return errors.New("send channel is full")
	}
}
