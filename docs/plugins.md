# 插件系统架构

Strix 的插件系统是一个强大且可扩展的机制，旨在实现功能模块的高度解耦和统一管理。通过这个系统，开发者可以轻松地实现新的功能模块（如日志、监控、追踪等），并让使用者以一种配置驱动的方式来使用它们。

## 核心概念

插件系统由三个核心概念组成：

1.  **插件 (Plugin)**: 插件是系统中实现特定功能的具体模块。每个插件都由其作者定义一个明确的接口。
2.  **工厂 (Factory)**: 工厂是一个实现了 `plugin.Factory` 接口的类型，它知道如何根据配置来创建和初始化一个具体的插件实例。它是连接管理器和插件实现的桥梁。
3.  **管理器 (Manager)**: 管理器是所有插件的中央注册中心和生命周期控制器。它负责在启动时，根据全局配置，使用注册的工厂来创建和初始化所有的插件实例。

## 组件详解

### `Factory` 接口

`plugin.Factory` 是每个插件实现者都必须实现的接口，它定义了插件的“元数据”和构造逻辑。

```go
package plugin

type Factory interface {
	// Type 返回插件的大类，如 "log", "metrics" 等。
	Type() Type
	// Name 返回该工厂具体实现的名称，如 "default", "prometheus" 等。
	Name() string
	// ConfigType 返回一个代表该插件配置的空结构体指针。
	// 管理器将使用这个结构体来解析配置。
	ConfigType() any
	// Setup 根据解析好的配置，创建并返回一个插件实例。
	Setup(any) (any, error)
}
```
开发者需要直接实现这个接口中的所有方法，以供 `Manager` 调用。

### `Manager`

`Manager` 是插件系统的核心，其主要职责包括：
-   **注册工厂**: 通过 `RegisterFactory(f Factory)` 方法，将不同插件的工厂注册到管理器中。
-   **初始化插件**: 通过 `SetupPlugins(conf map[string]any)` 方法，解析配置，找到对应的工厂，并调用其 `Setup` 方法来创建所有插件实例。所有创建的实例都以 `any` (`interface{}`) 的形式保存在 `Manager` 内部。
-   **提供实例**: 通过 `GetPlugin(typ Type, name string) (any, error)` 方法，提供一个已经初始化好的插件实例。**此方法返回的是 `any` 类型，需要使用者自行进行类型断言。**

## 完整工作流程

下面演示了如何实现并使用一个插件。

### 第一步：实现一个插件

假设我们要实现一个日志插件。

**1. 定义插件接口和实现**

```go
// in package mylogger
package mylogger

type Logger interface {
    Info(msg string)
}

type logger struct {
    level string
}

func (l *logger) Info(msg string) { /* ... */ }

// NewLogger creates a new logger instance.
func NewLogger(config *Config) (Logger, error) {
    return &logger{level: config.Level}, nil
}
```

**2. 实现 `plugin.Factory`**

```go
// in package mylogger
const PluginType plugin.Type = "log"

type Config struct {
    Level string `mapstructure:"level"`
}

type factory struct{}

func (f *factory) Type() plugin.Type {
    return PluginType
}

func (f *factory) Name() string {
    return "default"
}

func (f *factory) ConfigType() any {
    return &Config{}
}

func (f *factory) Setup(cfg any) (any, error) {
    // cfg 是已经由 manager 解析好的 Config 实例
    loggerConfig := cfg.(*Config)
    return NewLogger(loggerConfig)
}

// init 函数中自动注册工厂
func init() {
    plugin.MustRegister(&factory{})
}
```

### 第二步：配置插件

在您的 TOML 配置文件中，按层级定义插件的配置。

```toml
# toml config
[plugin.log.default]
level = "info"

[plugin.log.another]
# tag 可以在获取时替代 map 的 key "another"
tag = "file_logger"
level = "debug"
```

### 第三步：使用插件

在应用初始化时，设置 `Manager`。在业务逻辑中，从 `Manager` 获取插件并**手动进行类型断言**。

```go
package main

import (
    "fmt"
    "github.com/your-repo/strix/plugin"
    "github.com/your-repo/strix/plugins/mylogger" // 导入插件包以执行 init() 注册工厂
)


func main() {
    // 1. 创建 Manager
    manager := plugin.NewManager()
    
    // 2. 加载配置并设置插件
    // tomlConfig 是从文件加载并解析的 map[string]any
    var tomlConfig map[string]any 
    if err := manager.SetupPlugins(tomlConfig["plugin"].(map[string]any)); err != nil {
        panic(err)
    }
    
    // 3. 从 Manager 获取插件实例
    instance, err := manager.GetPlugin(mylogger.PluginType, "default")
    if err != nil {
        panic(err)
    }

    // 4. 对 any 类型的实例进行手动类型断言
    // 这是获取插件最关键的一步，必须检查 'ok' 以防 panic
    defaultLogger, ok := instance.(mylogger.Logger)
    if !ok {
        panic(fmt.Sprintf("plugin type assertion failed: expected %T", defaultLogger))
    }

    // 5. 安全地使用插件
    defaultLogger.Info("Server starting...")
}
```
**重点**: 类型断言 `instance.(mylogger.Logger)` 会返回两个值，第二个布尔值 `ok` 代表断言是否成功。**必须检查 `ok` 的值**，如果为 `false` 则表示 `instance` 并非您期望的类型，直接使用会导致程序 `panic`。
