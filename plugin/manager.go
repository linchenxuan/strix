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
	// ErrFactoryDestroy 表示工厂销毁插件实例失败。
	ErrFactoryDestroy = errors.New("factory destroy error")
)

// initializedPlugin 记录一次 setup 成功创建的插件实例及其工厂信息。
type initializedPlugin struct {
	pluginType  Type
	instanceKey string
	factoryName string
	factory     Factory
	plugin      Plugin
}

// manager 负责管理插件工厂与插件实例。
type manager struct {
	// factories: 插件类型 -> 工厂名 -> 工厂实现。
	factories map[Type]map[string]Factory
	// plugins: 插件类型 -> 实例键（name/tag）-> 插件实例。
	plugins map[Type]map[string]Plugin
	// mu 保护 factories 和 plugins 的并发访问。
	mu sync.RWMutex
}

var defaultManager = newManager()

// newManager 创建并返回一个新的 manager。
func newManager() *manager {
	return &manager{
		factories: make(map[Type]map[string]Factory),
		plugins:   make(map[Type]map[string]Plugin),
	}
}

// RegisterFactory 在默认全局管理器上注册插件工厂。
func RegisterFactory(factory Factory) {
	defaultManager.RegisterFactory(factory)
}

// SetupPlugins 在默认全局管理器上根据配置初始化所有插件。
func SetupPlugins(pluginConf Config) error {
	return defaultManager.SetupPlugins(pluginConf)
}

// GetPlugin 从默认全局管理器中获取插件实例。
func GetPlugin(pluginType Type, instanceName string) (Plugin, error) {
	return defaultManager.GetPlugin(pluginType, instanceName)
}

// GetDefaultPlugin 从默认全局管理器中获取默认插件实例。
func GetDefaultPlugin(pluginType Type) (Plugin, error) {
	return defaultManager.GetDefaultPlugin(pluginType)
}

// Destroy 销毁默认全局管理器中的所有插件实例。
func Destroy() error {
	return defaultManager.Destroy()
}

func resetManagerForTest() {
	defaultManager = newManager()
}

// RegisterFactory 注册插件工厂。
func (m *manager) RegisterFactory(factory Factory) {
	m.mu.Lock()
	defer m.mu.Unlock()

	factories, ok := m.factories[factory.Type()]
	if !ok {
		factories = make(map[string]Factory)
		m.factories[factory.Type()] = factories
	}
	factories[factory.Name()] = factory
}

func (m *manager) SetupPlugins(pluginConf Config) error {
	m.mu.Lock()
	initialized := make([]initializedPlugin, 0)

	for pluginType, factoryConfigs := range pluginConf {
		factories, ok := m.factories[pluginType]
		if !ok {
			continue
		}
		added, err := m.setupPluginsByType(pluginType, factoryConfigs, factories)
		initialized = append(initialized, added...)
		if err != nil {
			rollbackTargets := m.rollbackPlugins(initialized)
			m.mu.Unlock()
			rollbackErr := destroyPlugins(rollbackTargets)
			if rollbackErr != nil {
				return errors.Join(err, rollbackErr)
			}
			return err
		}
	}
	m.mu.Unlock()
	return nil
}

// setupPluginsByType 初始化某一插件类型下的所有插件实例。
func (m *manager) setupPluginsByType(pluginType Type, factoryConfigs TypeConfig, factories map[string]Factory) ([]initializedPlugin, error) {
	added := make([]initializedPlugin, 0, len(factoryConfigs))
	for factoryName, factoryConfig := range factoryConfigs {
		ins, err := m.setupSinglePlugin(pluginType, factoryName, factoryConfig, factories)
		if err != nil {
			return added, err
		}
		added = append(added, ins)
	}

	return added, nil
}

// setupSinglePlugin 初始化并注册一个插件实例。
func (m *manager) setupSinglePlugin(pluginType Type, factoryName string, factoryConfig RawConfig, factories map[string]Factory) (initializedPlugin, error) {
	factory, ok := factories[factoryName]
	if !ok {
		return initializedPlugin{}, fmt.Errorf("%w: plugin factory not found for type '%s' and name '%s'", ErrPluginNotFound, pluginType, factoryName)
	}

	targetConfig, err := decodeConfig(factory, pluginType, factoryName, factoryConfig)
	if err != nil {
		return initializedPlugin{}, err
	}

	pluginInstance, err := factory.Setup(targetConfig)
	if err != nil {
		return initializedPlugin{}, fmt.Errorf("%w: failed to setup plugin '%s':'%s': %v", ErrFactorySetup, pluginType, factoryName, err)
	}

	instanceKey := buildInstanceKey(factoryName, factoryConfig)
	if err := m.registerPlugin(pluginType, instanceKey, pluginInstance); err != nil {
		if destroyErr := safeDestroy(factory, pluginInstance); destroyErr != nil {
			return initializedPlugin{}, errors.Join(
				err,
				fmt.Errorf("%w: failed to destroy plugin '%s' of type '%s' after register failure: %v", ErrFactoryDestroy, instanceKey, pluginType, destroyErr),
			)
		}
		return initializedPlugin{}, err
	}
	return initializedPlugin{
		pluginType:  pluginType,
		instanceKey: instanceKey,
		factoryName: factoryName,
		factory:     factory,
		plugin:      pluginInstance,
	}, nil
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
func (m *manager) registerPlugin(pluginType Type, instanceKey string, pluginInstance Plugin) error {
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
func (m *manager) GetPlugin(pluginType Type, instanceName string) (Plugin, error) {
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
func (m *manager) GetDefaultPlugin(pluginType Type) (Plugin, error) {
	return m.GetPlugin(pluginType, DefaultInstanceName)
}

// Destroy 销毁并清空当前已初始化的所有插件实例。
func (m *manager) Destroy() error {
	m.mu.Lock()
	destroyTargets, collectErr := m.collectDestroyTargets()
	m.plugins = make(map[Type]map[string]Plugin)
	m.mu.Unlock()

	destroyErr := destroyPlugins(destroyTargets)
	if collectErr != nil && destroyErr != nil {
		return errors.Join(collectErr, destroyErr)
	}
	if collectErr != nil {
		return collectErr
	}
	return destroyErr
}

// rollbackPlugins 回滚本次 SetupPlugins 已创建的实例。
// 调用方必须持有 m.mu 写锁。
func (m *manager) rollbackPlugins(initialized []initializedPlugin) []initializedPlugin {
	targets := make([]initializedPlugin, 0, len(initialized))
	for i := len(initialized) - 1; i >= 0; i-- {
		ins := initialized[i]
		if pluginsByType, ok := m.plugins[ins.pluginType]; ok {
			delete(pluginsByType, ins.instanceKey)
			if len(pluginsByType) == 0 {
				delete(m.plugins, ins.pluginType)
			}
		}
		targets = append(targets, ins)
	}
	return targets
}

// collectDestroyTargets 收集销毁目标并校验对应工厂是否存在。
// 调用方必须持有 m.mu 写锁。
func (m *manager) collectDestroyTargets() ([]initializedPlugin, error) {
	targets := make([]initializedPlugin, 0)
	var errs []error

	for pluginType, instances := range m.plugins {
		factoriesByType := m.factories[pluginType]
		for instanceKey, pluginInstance := range instances {
			if pluginInstance == nil {
				errs = append(errs, fmt.Errorf("%w: plugin '%s' of type '%s' is nil", ErrPluginNotFound, instanceKey, pluginType))
				continue
			}

			factoryName := pluginInstance.FactoryName()
			factory, ok := factoriesByType[factoryName]
			if !ok {
				errs = append(errs, fmt.Errorf("%w: plugin factory '%s' not found for plugin '%s' of type '%s'", ErrPluginNotFound, factoryName, instanceKey, pluginType))
				continue
			}

			targets = append(targets, initializedPlugin{
				pluginType:  pluginType,
				instanceKey: instanceKey,
				factoryName: factoryName,
				factory:     factory,
				plugin:      pluginInstance,
			})
		}
	}

	return targets, errors.Join(errs...)
}

func destroyPlugins(targets []initializedPlugin) error {
	var errs []error
	for _, target := range targets {
		if err := safeDestroy(target.factory, target.plugin); err != nil {
			errs = append(errs, fmt.Errorf(
				"%w: failed to destroy plugin '%s' of type '%s' with factory '%s': %v",
				ErrFactoryDestroy, target.instanceKey, target.pluginType, target.factoryName, err,
			))
		}
	}
	return errors.Join(errs...)
}

func safeDestroy(factory Factory, plugin Plugin) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	factory.Destroy(plugin)
	return nil
}
