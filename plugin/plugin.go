package plugin

// Type 表示系统支持的插件类型。
type Type string

// RawConfig 表示单个插件实例的原始配置。
type RawConfig map[string]any

// TypeConfig 表示某个插件类型下的实例配置集合。
// key 为插件工厂名（Factory.Name）。
type TypeConfig map[string]RawConfig

// Config 表示 [plugin] 节点的完整配置。
// key 为插件类型（Type）。
type Config map[Type]TypeConfig

// Factory 定义了插件工厂的标准接口。
type Factory interface {
	// Type 返回插件类型。
	Type() Type
	// Name 返回插件实现名称。
	Name() string
	// ConfigType 返回配置结构体指针，插件管理器会使用 mapstructure 填充该结构体。
	ConfigType() any
	// Setup 根据配置初始化插件实例。
	// config 的实际类型与 ConfigType 返回的结构体类型一致。
	Setup(any) (Plugin, error)

	// Destroy 负责优雅关闭插件实例，释放连接、文件句柄、协程等资源。
	Destroy(Plugin)
}

// Plugin 是所有插件实例的标记接口。
// FactoryName 返回创建该插件的工厂名称，用于运行时识别具体实现。
type Plugin interface {
	FactoryName() string
}
