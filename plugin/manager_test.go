package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockConfig is a mock configuration struct for testing structured config.
type MockConfig struct {
	Level  string
	Output string
	Tag    string // Used for duplicate tag testing
}

// MockFactory is a mock implementation of the Factory interface for testing.
type MockFactory struct {
	PType Type
	PName string
	// Test helpers
	SetupCount   int
	DestroyCount int
}

func (m *MockFactory) Type() Type   { return m.PType }
func (m *MockFactory) Name() string { return m.PName }
func (m *MockFactory) ConfigType() any {
	return &MockConfig{} // Return an empty mock config struct
}
func (m *MockFactory) Setup(config any) (Plugin, error) {
	m.SetupCount++
	// In a real scenario, you'd assert config to *MockConfig and use its values
	return &MockPlugin{FName: m.PName}, nil
}
func (m *MockFactory) Destroy(p Plugin) {
	m.DestroyCount++
}

// MockPlugin is a mock plugin instance for testing.
type MockPlugin struct {
	FName string
}

func (mp *MockPlugin) FactoryName() string {
	return mp.FName
}

const (
	Log Type = "log"
)

func TestManager(t *testing.T) {
	factory := &MockFactory{PType: Log, PName: "mocklogger"}

	t.Run("RegisterFactory", func(t *testing.T) {
		manager := NewManager()
		manager.RegisterFactory(factory)
		assert.NotNil(t, manager.factories[Log])
		assert.Equal(t, factory, manager.factories[Log]["mocklogger"])
	})

	t.Run("SetupAndGetPlugins", func(t *testing.T) {
		manager := NewManager()

		pluginConf := map[string]any{
			"log": map[string]any{
				"mocklogger": map[string]any{
					"level":  "info",
					"output": "/var/log/mock.log",
					"tag":    "default",
				},
				"anotherlogger": map[string]any{
					"level": "debug",
				},
			},
		}

		anotherFactory := &MockFactory{PType: Log, PName: "anotherlogger"}
		manager.RegisterFactory(anotherFactory)
		manager.RegisterFactory(factory)

		err := manager.SetupPlugins(pluginConf)
		assert.NoError(t, err)

		p, err := manager.GetPlugin(Log, "default")
		assert.NoError(t, err)
		assert.NotNil(t, p)
		assert.IsType(t, &MockPlugin{}, p)

		// GetDefaultPlugin still returns `any`, so IsType is still appropriate here
		dp, err := manager.GetDefaultPlugin(Log)
		assert.NoError(t, err)
		assert.IsType(t, &MockPlugin{}, dp)
		assert.Equal(t, p, dp)

		np, err := manager.GetPlugin(Log, "anotherlogger")
		assert.NoError(t, err)
		assert.NotNil(t, np)
	})

	t.Run("ErrorOnDuplicateTag", func(t *testing.T) {
		manager := NewManager()

		manager.RegisterFactory(&MockFactory{PType: Log, PName: "logger1"})
		manager.RegisterFactory(&MockFactory{PType: Log, PName: "logger2"})

		pluginConf := map[string]any{
			"log": map[string]any{
				"logger1": map[string]any{
					"tag": "default",
				},
				"logger2": map[string]any{
					"tag": "default", // Duplicate tag
				},
			},
		}

		err := manager.SetupPlugins(pluginConf)
		assert.ErrorIs(t, err, ErrDuplicatePlugin) // Verify custom error
	})

	t.Run("ErrorOnMissingFactory", func(t *testing.T) {
		manager := NewManager()

		manager.RegisterFactory(&MockFactory{PType: Log, PName: "reallogger"})

		pluginConf := map[string]any{
			"log": map[string]any{
				"nonexistent": map[string]any{},
			},
		}
		err := manager.SetupPlugins(pluginConf)
		assert.ErrorIs(t, err, ErrPluginNotFound) // Verify custom error
	})

	t.Run("TestConfigDecoding", func(t *testing.T) {
		// New manager for this subtest
		manager := NewManager()

		// Register a factory that expects MockConfig
		mockConfigFactory := &MockFactory{PType: "test", PName: "mockconfigfactory"}
		manager.RegisterFactory(mockConfigFactory) // Registered here once for TestConfigDecoding

		t.Run("SuccessfulDecoding", func(t *testing.T) {
			pluginConf := map[string]any{
				"test": map[string]any{
					"mockconfigfactory": map[string]any{
						"Level":  "info",
						"Output": "stdout",
					},
				},
			}
			err := manager.SetupPlugins(pluginConf) // This should now succeed
			assert.NoError(t, err)

			// Get the plugin and ensure Setup received the correct config
			// (Cannot directly verify received config in mock setup without more complex mocking,
			// but we can ensure no decoding errors occurred)
		})

		t.Run("FailedDecoding_InvalidType", func(t *testing.T) {
			// Need a fresh manager and factory registration for this subtest
			// so that SetupPlugins can be called without ErrDuplicatePlugin
			subManager := NewManager()
			subManager.RegisterFactory(mockConfigFactory)

			pluginConf := map[string]any{
				"test": map[string]any{
					"mockconfigfactory": map[string]any{
						"Level":  123, // Invalid type for string
						"Output": "stdout",
					},
				},
			}
			err := subManager.SetupPlugins(pluginConf)
			assert.Error(t, err)
			assert.ErrorIs(t, err, ErrConfigDecode)
		})

		t.Run("FailedDecoding_InvalidFormat", func(t *testing.T) {
			// Need a fresh manager and factory registration for this subtest
			subManager := NewManager()
			subManager.RegisterFactory(mockConfigFactory)

			pluginConf := map[string]any{
				"test": "not-a-map", // Invalid format for plugins map
			}
			err := subManager.SetupPlugins(pluginConf)
			assert.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidConfigFormat)
		})
	})
}
