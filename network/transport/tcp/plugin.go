package tcp

import (
	"errors"
	"fmt"

	"github.com/linchenxuan/strix/plugin"
)

type factory struct{}

var _ plugin.Factory = (*factory)(nil)

// NewFactory creates a TCP transport plugin factory.
func NewFactory() plugin.Factory {
	return &factory{}
}

// Type returns the plugin type.
func (f *factory) Type() plugin.Type {
	return plugin.CSTransport
}

// Name returns the factory name used by plugin config.
func (f *factory) Name() string {
	return "tcp_transport"
}

// ConfigType returns the config type for mapstructure decoding.
func (f *factory) ConfigType() any {
	return &TCPTransportCfg{}
}

// Setup initializes a TCP transport plugin instance.
func (f *factory) Setup(cfgAny any) (plugin.Plugin, error) {
	cfg, ok := cfgAny.(*TCPTransportCfg)
	if !ok {
		return nil, errors.New("tcp setup failed: invalid config type")
	}

	ins, err := NewTCPTransport(cfg)
	if err != nil {
		return nil, fmt.Errorf("tcp setup failed: %w", err)
	}
	return ins, nil
}

// Destroy gracefully shuts down the TCP transport plugin.
func (f *factory) Destroy(p plugin.Plugin) {
	if tp, ok := p.(*TCPTransport); ok && tp != nil {
		_ = tp.Stop()
	}
}
