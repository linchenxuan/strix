package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockConfig 是用于测试结构化配置解析的模拟配置。
type MockConfig struct {
	Level  string
	Output string
	Tag    string // 用于测试重复 tag 场景。
}

// MockFactory 是 Factory 接口的测试桩实现。
type MockFactory struct {
	PluginType Type
	ImplName   string
	// 测试统计字段。
	SetupCount   int
	DestroyCount int
}

func (m *MockFactory) Type() Type   { return m.PluginType }
func (m *MockFactory) Name() string { return m.ImplName }
func (m *MockFactory) ConfigType() any {
	return &MockConfig{}
}
func (m *MockFactory) Setup(config any) (Plugin, error) {
	m.SetupCount++
	return &MockPlugin{Factory: m.ImplName}, nil
}
func (m *MockFactory) Destroy(p Plugin) {
	m.DestroyCount++
}

// NilConfigFactory 用于测试 ConfigType 返回 nil 的场景。
type NilConfigFactory struct {
	PluginType Type
	ImplName   string
}

func (f *NilConfigFactory) Type() Type      { return f.PluginType }
func (f *NilConfigFactory) Name() string    { return f.ImplName }
func (f *NilConfigFactory) ConfigType() any { return nil }
func (f *NilConfigFactory) Setup(config any) (Plugin, error) {
	return &MockPlugin{Factory: f.ImplName}, nil
}
func (f *NilConfigFactory) Destroy(p Plugin) {}

// MockPlugin 是测试用的插件实例。
type MockPlugin struct {
	Factory string
}

func (mp *MockPlugin) FactoryName() string {
	return mp.Factory
}

const (
	logType Type = "log"
)

func TestManager(t *testing.T) {
	factory := &MockFactory{PluginType: logType, ImplName: "mocklogger"}

	t.Run("RegisterFactory", func(t *testing.T) {
		manager := NewManager()
		manager.RegisterFactory(factory)
		assert.NotNil(t, manager.factories[logType])
		assert.Equal(t, factory, manager.factories[logType]["mocklogger"])
	})

	t.Run("SetupAndGetPlugins", func(t *testing.T) {
		manager := NewManager()

		pluginConf := Config{
			logType: TypeConfig{
				"mocklogger": RawConfig{
					"level":  "info",
					"output": "/var/log/mock.log",
					"tag":    "default",
				},
				"anotherlogger": RawConfig{
					"level": "debug",
				},
			},
		}

		anotherFactory := &MockFactory{PluginType: logType, ImplName: "anotherlogger"}
		manager.RegisterFactory(anotherFactory)
		manager.RegisterFactory(factory)

		err := manager.SetupPlugins(pluginConf)
		assert.NoError(t, err)

		p, err := manager.GetPlugin(logType, "default")
		assert.NoError(t, err)
		assert.NotNil(t, p)
		assert.IsType(t, &MockPlugin{}, p)

		// 这里验证默认实例与按 tag 获取到的是同一个对象。
		dp, err := manager.GetDefaultPlugin(logType)
		assert.NoError(t, err)
		assert.IsType(t, &MockPlugin{}, dp)
		assert.Equal(t, p, dp)

		np, err := manager.GetPlugin(logType, "anotherlogger")
		assert.NoError(t, err)
		assert.NotNil(t, np)
	})

	t.Run("ErrorOnDuplicateTag", func(t *testing.T) {
		manager := NewManager()

		manager.RegisterFactory(&MockFactory{PluginType: logType, ImplName: "logger1"})
		manager.RegisterFactory(&MockFactory{PluginType: logType, ImplName: "logger2"})

		pluginConf := Config{
			logType: TypeConfig{
				"logger1": RawConfig{
					"tag": "default",
				},
				"logger2": RawConfig{
					"tag": "default", // 重复 tag。
				},
			},
		}

		err := manager.SetupPlugins(pluginConf)
		assert.ErrorIs(t, err, ErrDuplicatePlugin)
	})

	t.Run("ErrorOnMissingFactory", func(t *testing.T) {
		manager := NewManager()

		manager.RegisterFactory(&MockFactory{PluginType: logType, ImplName: "reallogger"})

		pluginConf := Config{
			logType: TypeConfig{
				"nonexistent": RawConfig{},
			},
		}
		err := manager.SetupPlugins(pluginConf)
		assert.ErrorIs(t, err, ErrPluginNotFound)
	})

	t.Run("TestConfigDecoding", func(t *testing.T) {
		manager := NewManager()

		mockConfigFactory := &MockFactory{PluginType: "test", ImplName: "mockconfigfactory"}
		manager.RegisterFactory(mockConfigFactory)

		t.Run("SuccessfulDecoding", func(t *testing.T) {
			pluginConf := Config{
				Type("test"): TypeConfig{
					"mockconfigfactory": RawConfig{
						"Level":  "info",
						"Output": "stdout",
					},
				},
			}
			err := manager.SetupPlugins(pluginConf)
			assert.NoError(t, err)
		})

		t.Run("FailedDecoding_InvalidType", func(t *testing.T) {
			// 使用独立 manager，避免重复初始化同名插件。
			subManager := NewManager()
			subManager.RegisterFactory(mockConfigFactory)

			pluginConf := Config{
				Type("test"): TypeConfig{
					"mockconfigfactory": RawConfig{
						"Level":  123, // string 字段给了 int，触发解码错误。
						"Output": "stdout",
					},
				},
			}
			err := subManager.SetupPlugins(pluginConf)
			assert.Error(t, err)
			assert.ErrorIs(t, err, ErrConfigDecode)
		})

		t.Run("FailedConfigType_Nil", func(t *testing.T) {
			subManager := NewManager()
			subManager.RegisterFactory(&NilConfigFactory{PluginType: "test", ImplName: "nilconfigfactory"})

			pluginConf := Config{
				Type("test"): TypeConfig{
					"nilconfigfactory": RawConfig{},
				},
			}
			err := subManager.SetupPlugins(pluginConf)
			assert.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidFactoryConfig)
		})
	})
}
