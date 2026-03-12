package tracing

import (
	"github.com/linchenxuan/strix/log"
	"github.com/linchenxuan/strix/plugin"
)

// tracerPlugin 包装 Tracer 以满足 plugin.Plugin 接口。
type tracerPlugin struct {
	tracer *Tracer // 被管理的 Tracer 实例。
}

// FactoryName 返回插件工厂名称。
func (p *tracerPlugin) FactoryName() string {
	return PluginName
}

// factory 实现 plugin.Factory 接口。
type factory struct{}

// Type 返回插件类型。
func (f *factory) Type() plugin.Type {
	return PluginType
}

// Name 返回插件名称。
func (f *factory) Name() string {
	return PluginName
}

// ConfigType 返回配置结构体指针，供插件管理器反序列化。
func (f *factory) ConfigType() any {
	return &Config{}
}

// Setup 根据配置初始化 Tracer 插件实例。
func (f *factory) Setup(rawCfg any) (plugin.Plugin, error) {
	cfg := rawCfg.(*Config)

	// 使用 tracing 专用的 Logger 配置创建独立日志器。
	logger := log.NewLogger(&cfg.LogCfg)

	t := NewTracer(*cfg, logger)
	SetTracer(t)

	log.Info().
		Uint16("serverId", cfg.ServerID).
		Uint32("sampleRate", cfg.SampleRate).
		Int("ringBufferPower", cfg.RingBufferPower).
		Str("path", cfg.LogCfg.LogPath).
		Msg("Init ring buffer tracer")

	return &tracerPlugin{tracer: t}, nil
}

// Destroy 停止 Tracer，刷新剩余数据。
func (f *factory) Destroy(p plugin.Plugin) {
	tp, ok := p.(*tracerPlugin)
	if !ok || tp.tracer == nil {
		return
	}
	tp.tracer.Stop()
	SetTracer(nil)
	log.Info().Msg("Destroy ring buffer tracer")
}

func init() {
	plugin.RegisterFactory(&factory{})
}
