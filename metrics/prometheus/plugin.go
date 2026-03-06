package prometheus

import (
	"fmt"

	"github.com/linchenxuan/strix/log"
	"github.com/linchenxuan/strix/plugin"
)

type factory struct{}

// Type 返回插件类型。
func (f *factory) Type() plugin.Type {
	return plugin.Type("metrics")
}

// Name 返回插件实现名称。
func (f *factory) Name() string {
	return "prometheus"
}

// ConfigType 返回插件配置结构体指针。
// Manager 会使用 mapstructure 将原始配置填充到该结构体。
func (f *factory) ConfigType() any {
	return &PrometheusReporterConfig{}
}

// Setup 根据配置初始化插件实例。
func (f *factory) Setup(cfgAny any) (plugin.Plugin, error) {
	cfg, ok := cfgAny.(*PrometheusReporterConfig)
	if !ok {
		return nil, fmt.Errorf("prometheus setup failed: invalid config type %T", cfgAny)
	}
	if cfg == nil {
		return nil, fmt.Errorf("prometheus setup failed: nil config")
	}

	if cfg.MetricPath == "" {
		return nil, fmt.Errorf("prometheus setup failed: metricPath is required")
	}
	if cfg.EnableHealthCheck && cfg.HealthCheckPath == "" {
		return nil, fmt.Errorf("prometheus setup failed: healthCheckPath is required when enableHealthCheck is true")
	}
	if cfg.UsePush {
		if cfg.PushAddr == "" {
			return nil, fmt.Errorf("prometheus setup failed: pushAddr is required when usePush is true")
		}
		if cfg.PushIntervalSec <= 0 {
			return nil, fmt.Errorf("prometheus setup failed: pushIntervalSec must be greater than 0 when usePush is true")
		}
		if cfg.PushJobName == "" {
			return nil, fmt.Errorf("prometheus setup failed: pushJobName is required when usePush is true")
		}
	}

	p := NewPrometheusReporter()
	p.cfg = cfg

	if err := p.start(); err != nil {
		p.stop()
		return nil, fmt.Errorf("prometheus setup failed: %w", err)
	}

	return p, nil
}

func (f *factory) Destroy(p plugin.Plugin) {
	prom, ok := p.(*PrometheusReporter)
	if !ok || prom == nil {
		log.Error().Str("pluginType", fmt.Sprintf("%T", p)).Msg("prometheus destroy failed: invalid plugin type")
		return
	}

	prom.stop()
}
