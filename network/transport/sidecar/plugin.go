package sidecar

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/linchenxuan/strix/log"
	"github.com/linchenxuan/strix/network/pb"
	"github.com/linchenxuan/strix/network/transport"
	"github.com/linchenxuan/strix/plugin"
	"github.com/linchenxuan/strix/runtime"
	"github.com/linchenxuan/strix/utils/shm"
)

type factory struct{}
var _ plugin.Factory = (*factory)(nil)

// NewFactory creates a sidecar transport plugin factory.
func NewFactory() plugin.Factory {
	return &factory{}
}

// Type returns the plugin type.
func (s *factory) Type() plugin.Type {
	return plugin.SSTransport
}

// Name returns the name of the plugin implementation.
func (r *factory) Name() string {
	return "sidecar_transport"
}

// ConfigType returns an empty struct that represents the plugin's configuration.
// This struct will be populated by the manager using mapstructure.
func (r *factory) ConfigType() any {
	return &SidecarConfig{}
}

// Setup initializes a plugin instance based on the configuration.
func (r *factory) Setup(cfgAny any) (plugin.Plugin, error) {
	log.Info().Msg("Sidecar Factory Setup")

	cfg, ok := cfgAny.(*SidecarConfig)
	if !ok {
		return nil, errors.New("sidecar setup failed: invalid config type")
	}

	sidecar := &Sidecar{
		svrState:         int32(pb.ESvrState_ST_HEALTH),
		chUpdateSvrState: make(chan struct{}, 1),
		EncoderPool:      transport.NewEncoderPool(),
		stopRecv:         make(chan bool, 1),
		closeUnixSocket:  make(chan struct{}, 1),
		pkgChan:          make(chan sidecarMsg, 10),
		wg:               &sync.WaitGroup{},
		recvWG:           &sync.WaitGroup{},
		cfg:              cfg,
	}

	// 1. Configuration parsing & module initialization
	sidecar.initMeshAddr()
	if sidecar.state != _stateNone {
		return nil, errors.New("sidecar setup failed: invalid initial sidecar state")
	}

	// 2. File lock
	defer func() {
		if sidecar.state == _stateNone {
			if err := sidecar.ss.UnlockShmPipe(); err != nil {
				log.Error().Err(err).Msg("Sidecar Factory Setup filelock close err")
			}
		}
	}()
	if sidecar.shmpipeIdx = shm.GetAvailableShmpipeIdx(sidecar.cfg.getShmDirPath(),
		sidecar.cfg.getFileLockName(), sidecar.cfg.getShmpipeCnt(),
		uint64(runtime.GetSvrBuildTime())); sidecar.shmpipeIdx == -1 {
		return nil, errors.New("get available shmpipe idx fail")
	}

	if err := sidecar.ss.LockShmpipe(sidecar.cfg.getShmDirPath(), sidecar.cfg.getFileLockName(),
		sidecar.shmpipeIdx, uint64(runtime.GetSvrBuildTime())); err != nil {
		return nil, err
	}

	// 3. Attach shared memory
	defer func() {
		if sidecar.state == _stateNone {
			sidecar.shmChannel.clean()
		}
	}()
	timeout := _waitFileTimeout
	if err := sidecar.shmChannel.init(sidecar.cfg.getRecvDataFilePath(sidecar.shmpipeIdx),
		sidecar.cfg.getSendDataFilePath(sidecar.shmpipeIdx), &timeout); err != nil {
		return nil, fmt.Errorf("Sidecar shmChannel init: %w", err)
	}
	log.Info().Str("readShmFilePath", sidecar.cfg.getRecvDataFilePath(sidecar.shmpipeIdx)).
		Str("writeShmFilePath", sidecar.cfg.getSendDataFilePath(sidecar.shmpipeIdx)).Msg("shmchannel init succ")
	sidecar.state = _stateInited

	// 4. Register with consul
	sidecar.wg.Add(1)
	started := &sync.WaitGroup{}
	started.Add(1)
	sidecar.ctx, sidecar.cancel = context.WithCancel(context.Background())
	go sidecar.run(sidecar.ctx, started)

	go sidecar.ss.RenewShmpipePeriodically(sidecar.ctx, sidecar.stopRecv)

	started.Wait()

	log.Info().Msg("Sidecar Setup Succeed")

	return sidecar, nil
}

func (r *factory) Destroy(p plugin.Plugin) {
	log.Info().Msg("Sidecar Factory Destroy")

	sidecar := p.(*Sidecar)

	if err := sidecar.Stop(); err != nil {
		log.Error().Err(err).Msg("Sidecar Factory Destroy")
	}
}
