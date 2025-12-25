package zipking

import (
	"errors"

	"github.com/linchenxuan/strix/log"
	"github.com/linchenxuan/strix/plugin"
)

// Type returns the plugin type.
func (r *ZipkinReporter) Type() plugin.Type {
	return plugin.Tracer
}

// Name returns the name of the plugin implementation.
func (r *ZipkinReporter) Name() string {
	return "prometheus"
}

// ConfigType returns an empty struct that represents the plugin's configuration.
// This struct will be populated by the manager using mapstructure.
func (r *ZipkinReporter) ConfigType() any {
	return &ZipkinReporterConfig{}
}

// Setup initializes a plugin instance based on the configuration.
func (r *ZipkinReporter) Setup(cfgAny any) (any, error) {
	cfg, ok := cfgAny.(*ZipkinReporterConfig)
	if !ok {
		log.Fatal().Msg("prometheus setup failed")
		return nil, errors.New("prometheus setup failed")
	}

	p := newZipkinReporter(cfg)
	p.cfg = cfg

	p.start()

	return p, nil
}

func (r *ZipkinReporter) Destroy() {

}
