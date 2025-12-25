package plugin

import (
	"errors" // Import errors package
	"fmt"
	"sync"

	"github.com/mitchellh/mapstructure"
)

const (
	// DefaultInsName is the tag for the default plugin instance.
	DefaultInsName = "default"
)

var (
	ErrPluginNotFound      = errors.New("plugin not found")
	ErrDuplicatePlugin     = errors.New("duplicate plugin")
	ErrInvalidConfigFormat = errors.New("invalid config format")
	ErrConfigDecode        = errors.New("config decode error") // New error type for config decoding
	ErrFactorySetup        = errors.New("factory setup error") // New error type for factory setup
)

// Manager is responsible for managing all plugins in the framework.
type Manager struct {
	factories map[Type]map[string]Factory
	plugins   map[Type]map[string]Plugin
	lock      sync.RWMutex
}

// NewManager creates and returns a new Manager instance.
func NewManager() *Manager {
	return &Manager{
		factories: make(map[Type]map[string]Factory),
		plugins:   make(map[Type]map[string]Plugin),
	}
}

// RegisterFactory registers a plugin factory with the manager.
func (m *Manager) RegisterFactory(f Factory) {
	m.lock.Lock()
	defer m.lock.Unlock()

	factories, ok := m.factories[f.Type()]
	if !ok {
		factories = make(map[string]Factory)
		m.factories[f.Type()] = factories
	}
	factories[f.Name()] = f
}

// SetupPlugins sets up and initializes all plugins from the configuration.
// Here `pluginConf` should be the `[plugin]` part parsed from TOML.
func (m *Manager) SetupPlugins(pluginConf map[string]any) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	for typeName, plugins := range pluginConf {
		pluginType := Type(typeName)
		factories, ok := m.factories[pluginType]
		if !ok {
			continue // If no factory of this type is registered, it can be ignored or logged.
		}

		pluginsMap, ok := plugins.(map[string]any)
		if !ok {
			return fmt.Errorf("%w for plugin type '%s'", ErrInvalidConfigFormat, pluginType)
		}

		for name, config := range pluginsMap {
			factory, ok := factories[name]
			if !ok {
				return fmt.Errorf("%w: plugin factory not found for type '%s' and name '%s'", ErrPluginNotFound, pluginType, name)
			}

			configMap, ok := config.(map[string]any)
			if !ok {
				return fmt.Errorf("%w for plugin '%s':'%s'", ErrInvalidConfigFormat, pluginType, name)
			}

			// Get the target config struct from the factory
			targetConfig := factory.ConfigType()
			if targetConfig == nil {
				return fmt.Errorf("%w: plugin factory '%s':'%s' did not provide a configuration type", ErrInvalidConfigFormat, pluginType, name)
			}

			// Decode the raw config map into the structured config
			decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
				WeaklyTypedInput: false,
				Result:           targetConfig,
			})
			if err != nil {
				return fmt.Errorf("%w: failed to create config decoder for plugin '%s':'%s': %v", ErrConfigDecode, pluginType, name, err)
			}
			if err := decoder.Decode(configMap); err != nil {
				return fmt.Errorf("%w: failed to decode config for plugin '%s':'%s': %v", ErrConfigDecode, pluginType, name, err)
			}

			// Pass the structured config to the factory's Setup method
			ins, err := factory.Setup(targetConfig)
			if err != nil {
				return fmt.Errorf("%w: failed to setup plugin '%s':'%s': %v", ErrFactorySetup, pluginType, name, err)
			}

			if _, ok := m.plugins[pluginType]; !ok {
				m.plugins[pluginType] = make(map[string]Plugin)
			}

			// Register instance
			// Prioritize using tag as the key
			key := name
			if tag, ok := configMap["tag"].(string); ok && tag != "" {
				key = tag
			}

			if _, exists := m.plugins[pluginType][key]; exists {
				return fmt.Errorf("%w: duplicate plugin tag/name '%s' for type '%s'", ErrDuplicatePlugin, key, pluginType)
			}
			m.plugins[pluginType][key] = ins
		}
	}
	return nil
}

// GetPlugin gets an initialized plugin instance from the manager.
// `name` can be the name of the plugin or its tag.
func (m *Manager) GetPlugin(typ Type, name string) (any, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	plugins, ok := m.plugins[typ]
	if !ok {
		return nil, fmt.Errorf("%w: no plugins found for type '%s'", ErrPluginNotFound, typ)
	}

	plugin, ok := plugins[name]
	if !ok {
		return nil, fmt.Errorf("%w: plugin '%s' not found for type '%s'", ErrPluginNotFound, name, typ)
	}
	return plugin, nil
}

// GetDefaultPlugin gets the default plugin instance of the specified type from the manager.
func (m *Manager) GetDefaultPlugin(typ Type) (any, error) {
	return m.GetPlugin(typ, DefaultInsName)
}
