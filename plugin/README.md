# Plugin 模块说明

`plugin` 包是 Strix 的插件注册与实例管理层，负责：

- 注册插件工厂（`Factory`）
- 根据配置初始化插件实例（`Plugin`）
- 通过类型 + 名称（或 `tag`）检索实例

## 核心接口

### Type

插件类型标识，由业务自行定义，例如：

- `const MetricsType plugin.Type = "metrics"`
- `const TracerType plugin.Type = "tracer"`

### Factory

每个插件实现都通过工厂接入：

- `Type() Type`：返回插件类型
- `Name() string`：返回实现名（配置中用这个名字匹配工厂）
- `ConfigType() any`：返回配置结构体指针，供 `mapstructure` 解码
- `Setup(any) (Plugin, error)`：根据配置创建插件实例
- `Destroy(Plugin)`：释放插件资源

### Plugin

插件实例接口：

- `FactoryName() string`：返回创建该实例的工厂名

## Manager 能力

`Manager` 提供以下能力：

- `RegisterFactory(f Factory)`：注册工厂
- `SetupPlugins(pluginConf Config) error`：按配置初始化插件
- `GetPlugin(pluginType Type, instanceName string) (Plugin, error)`：按类型 + 名称获取实例
- `GetDefaultPlugin(pluginType Type) (Plugin, error)`：获取默认实例（键为 `default`）
- `Destroy() error`：销毁并清空已初始化的插件实例

说明：

- `Manager` 内部使用读写锁，支持并发读取
- 同一插件类型下，实例键不能重复，否则返回 `ErrDuplicatePlugin`
- `SetupPlugins` 在初始化过程中若失败，会回滚并销毁本次已成功创建的实例

## 配置结构

`SetupPlugins` 接收的数据应对应配置文件中的 `[plugin]` 节点，逻辑结构如下：

```text
pluginConf
  └── <pluginType>
      └── <factoryName>
          └── <config map>
```

字段语义：

- `<pluginType>`：插件类型（如 `metrics`）
- `<factoryName>`：必须与工厂 `Name()` 一致
- `<config map>`：传给工厂的配置
- `tag`（可选）：实例注册键；未配置时使用 `<factoryName>`

对应类型别名：

- `RawConfig`：单个插件实例配置（`map[string]any`）
- `TypeConfig`：同一类型下的实例配置集合（`map[string]RawConfig`）
- `Config`：整个 plugin 节点配置（`map[Type]TypeConfig`）

示例（伪代码）：

```go
pluginConf := plugin.Config{
	plugin.Type("metrics"): plugin.TypeConfig{
		"prometheus": plugin.RawConfig{
			"addr": ":9090",
			"tag":  "default",
		},
	},
}
```

## 初始化流程

```go
mgr := plugin.NewManager()

const metricsType plugin.Type = "metrics"

mgr.RegisterFactory(metricsPrometheusFactory)
mgr.RegisterFactory(tracerJaegerFactory)

if err := mgr.SetupPlugins(pluginConf); err != nil {
	// 处理初始化失败
}

ins, err := mgr.GetDefaultPlugin(metricsType)
if err != nil {
	// 处理获取失败
}
_ = ins

if err := mgr.Destroy(); err != nil {
	// 处理销毁失败
}
```

## 常见错误

- `ErrPluginNotFound`：插件或工厂不存在
- `ErrDuplicatePlugin`：同类型下出现重复实例键（`tag` 或 `name`）
- `ErrInvalidFactoryConfig`：工厂配置定义不合法（如工厂未提供配置结构体）
- `ErrConfigDecode`：配置解码失败
- `ErrFactorySetup`：工厂 `Setup` 执行失败
- `ErrFactoryDestroy`：工厂 `Destroy` 执行失败

## 新增一个插件实现

1. 定义配置结构体
2. 实现 `Factory`（含 `ConfigType` / `Setup` / `Destroy`）
3. 定义插件实例并实现 `Plugin`
4. 在启动阶段调用 `RegisterFactory`
5. 在配置中加入 `<pluginType>.<factoryName>` 节点
6. 通过 `GetPlugin` 或 `GetDefaultPlugin` 获取实例并使用
