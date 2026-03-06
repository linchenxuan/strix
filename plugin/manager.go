package plugin

import (
	"errors"
	"fmt"
	"sync"

	"github.com/mitchellh/mapstructure"
)

const (
	// DefaultInstanceName 是默认插件实例的标识。
	DefaultInstanceName = "default"
)

var (
	// ErrPluginNotFound 表示插件或工厂未找到。
	ErrPluginNotFound = errors.New("plugin not found")
	// ErrDuplicatePlugin 表示同一类型下出现重复插件键（name 或 tag）。
	ErrDuplicatePlugin = errors.New("duplicate plugin")
	// ErrInvalidFactoryConfig 表示插件工厂提供的配置定义不合法。
	ErrInvalidFactoryConfig = errors.New("invalid factory config")
	// ErrConfigDecode 表示插件配置反序列化失败。
	ErrConfigDecode = errors.New("config decode error")
	// ErrFactorySetup 表示工厂初始化插件实例失败。
	ErrFactorySetup = errors.New("factory setup error")
)

// Manager 负责管理插件工厂与插件实例。
type Manager struct {
	// factories: 插件类型 -> 工厂名 -> 工厂实现。
	factories map[Type]map[string]Factory
	// plugins: 插件类型 -> 实例键（name/tag）-> 插件实例。
	plugins map[Type]map[string]Plugin
	// mu 保护 factories 和 plugins 的并发访问。
	mu sync.RWMutex
}

// NewManager 创建并返回一个新的 Manager。
func NewManager() *Manager {
	return &Manager{
		factories: make(map[Type]map[string]Factory),
		plugins:   make(map[Type]map[string]Plugin),
	}
}

// RegisterFactory 注册插件工厂。
func (m *Manager) RegisterFactory(factory Factory) {
	m.mu.Lock()
	defer m.mu.Unlock()

	factories, ok := m.factories[factory.Type()]
	if !ok {
		factories = make(map[string]Factory)
		m.factories[factory.Type()] = factories
	}
	factories[factory.Name()] = factory
}

// SetupPlugins 根据配置初始化所有插件。
// pluginConf 对应配置文件中的 [plugin] 节点。
func (m *Manager) SetupPlugins(pluginConf Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for pluginType, factoryConfigs := range pluginConf {
		factories, ok := m.factories[pluginType]
		if !ok {
			continue
		}
		if err := m.setupPluginsByType(pluginType, factoryConfigs, factories); err != nil {
			return err
		}
	}
	return nil
}

// setupPluginsByType 初始化某一插件类型下的所有插件实例。
func (m *Manager) setupPluginsByType(pluginType Type, factoryConfigs TypeConfig, factories map[string]Factory) error {
	for factoryName, factoryConfig := range factoryConfigs {
		if err := m.setupSinglePlugin(pluginType, factoryName, factoryConfig, factories); err != nil {
			return err
		}
	}

	return nil
}

// setupSinglePlugin 初始化并注册一个插件实例。
func (m *Manager) setupSinglePlugin(pluginType Type, factoryName string, factoryConfig RawConfig, factories map[string]Factory) error {
	factory, ok := factories[factoryName]
	if !ok {
		return fmt.Errorf("%w: plugin factory not found for type '%s' and name '%s'", ErrPluginNotFound, pluginType, factoryName)
	}

	targetConfig, err := decodeConfig(factory, pluginType, factoryName, factoryConfig)
	if err != nil {
		return err
	}

	pluginInstance, err := factory.Setup(targetConfig)
	if err != nil {
		return fmt.Errorf("%w: failed to setup plugin '%s':'%s': %v", ErrFactorySetup, pluginType, factoryName, err)
	}

	instanceKey := buildInstanceKey(factoryName, factoryConfig)
	if err := m.registerPlugin(pluginType, instanceKey, pluginInstance); err != nil {
		return err
	}
	return nil
}

// decodeConfig 将原始配置解码到工厂声明的配置结构体中。
func decodeConfig(factory Factory, pluginType Type, factoryName string, factoryConfig RawConfig) (any, error) {
	targetConfig := factory.ConfigType()
	if targetConfig == nil {
		return nil, fmt.Errorf("%w: plugin factory '%s':'%s' did not provide a configuration type", ErrInvalidFactoryConfig, pluginType, factoryName)
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput: false,
		Result:           targetConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create config decoder for plugin '%s':'%s': %v", ErrConfigDecode, pluginType, factoryName, err)
	}
	if err := decoder.Decode(factoryConfig); err != nil {
		return nil, fmt.Errorf("%w: failed to decode config for plugin '%s':'%s': %v", ErrConfigDecode, pluginType, factoryName, err)
	}

	return targetConfig, nil
}

// buildInstanceKey 生成插件实例键。
// 优先使用 tag；未配置 tag 时回退到工厂名。
func buildInstanceKey(factoryName string, factoryConfig RawConfig) string {
	if tag, ok := factoryConfig["tag"].(string); ok && tag != "" {
		return tag
	}
	return factoryName
}

// registerPlugin 将实例写入插件注册表，并检查键冲突。
func (m *Manager) registerPlugin(pluginType Type, instanceKey string, pluginInstance Plugin) error {
	if _, ok := m.plugins[pluginType]; !ok {
		m.plugins[pluginType] = make(map[string]Plugin)
	}

	if _, exists := m.plugins[pluginType][instanceKey]; exists {
		return fmt.Errorf("%w: duplicate plugin tag/name '%s' for type '%s'", ErrDuplicatePlugin, instanceKey, pluginType)
	}
	m.plugins[pluginType][instanceKey] = pluginInstance
	return nil
}

// GetPlugin 获取已初始化的插件实例。
// instanceName 可以是插件名，也可以是 tag。
func (m *Manager) GetPlugin(pluginType Type, instanceName string) (Plugin, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugins, ok := m.plugins[pluginType]
	if !ok {
		return nil, fmt.Errorf("%w: no plugins found for type '%s'", ErrPluginNotFound, pluginType)
	}

	plugin, ok := plugins[instanceName]
	if !ok {
		return nil, fmt.Errorf("%w: plugin '%s' not found for type '%s'", ErrPluginNotFound, instanceName, pluginType)
	}
	return plugin, nil
}

// GetDefaultPlugin 获取指定类型的默认插件实例。
func (m *Manager) GetDefaultPlugin(pluginType Type) (Plugin, error) {
	return m.GetPlugin(pluginType, DefaultInstanceName)
}
