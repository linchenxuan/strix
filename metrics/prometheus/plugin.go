package prometheus

import (
	"github.com/linchenxuan/strix/log"
	"github.com/linchenxuan/strix/plugin"
)

type factory struct{}

// Type returns the plugin type.
func (f *factory) Type() plugin.Type {
	return plugin.Metrics
}

// Name returns the name of the plugin implementation.
func (f *factory) Name() string {
	return "prometheus"
}

// ConfigType returns an empty struct that represents the plugin's configuration.
// This struct will be populated by the manager using mapstructure.
func (f *factory) ConfigType() any {
	return &PrometheusReporterConfig{}
}

// Setup initializes a plugin instance based on the configuration.
func (f *factory) Setup(cfgAny any) (plugin.Plugin, error) {
	cfg, ok := cfgAny.(*PrometheusReporterConfig)
	if !ok {
		log.Fatal().Msg("prometheus setup failed")
	}

	p := NewPrometheusReporter()
	p.cfg = cfg

	p.start()

	return p, nil
}

func (f *factory) Destroy(p plugin.Plugin) {
	prom, ok := p.(*PrometheusReporter)
	if !ok {
		log.Fatal().Msg("prometheus destroy failed")
		return
	}

	prom.stop()
}
