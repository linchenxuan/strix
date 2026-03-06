package plugin

import (
	"errors"
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
	SetupErr     error
	DestroyPanic bool
}

func (m *MockFactory) Type() Type   { return m.PluginType }
func (m *MockFactory) Name() string { return m.ImplName }
func (m *MockFactory) ConfigType() any {
	return &MockConfig{}
}
func (m *MockFactory) Setup(config any) (Plugin, error) {
	if m.SetupErr != nil {
		return nil, m.SetupErr
	}
	m.SetupCount++
	return &MockPlugin{Factory: m.ImplName}, nil
}
func (m *MockFactory) Destroy(p Plugin) {
	if m.DestroyPanic {
		panic("mock destroy panic")
	}
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

		f1 := &MockFactory{PluginType: logType, ImplName: "logger1"}
		f2 := &MockFactory{PluginType: logType, ImplName: "logger2"}
		manager.RegisterFactory(f1)
		manager.RegisterFactory(f2)

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
		// 第二个插件 setup 成功但注册失败，应被即时销毁，避免泄漏。
		assert.Equal(t, 1, f2.DestroyCount)
		_, getErr := manager.GetPlugin(logType, "default")
		assert.ErrorIs(t, getErr, ErrPluginNotFound)
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

	t.Run("DestroyPlugins", func(t *testing.T) {
		manager := NewManager()
		f1 := &MockFactory{PluginType: logType, ImplName: "logger1"}
		f2 := &MockFactory{PluginType: logType, ImplName: "logger2"}
		manager.RegisterFactory(f1)
		manager.RegisterFactory(f2)

		pluginConf := Config{
			logType: TypeConfig{
				"logger1": RawConfig{"tag": "default"},
				"logger2": RawConfig{"tag": "logger2"},
			},
		}
		err := manager.SetupPlugins(pluginConf)
		assert.NoError(t, err)

		err = manager.Destroy()
		assert.NoError(t, err)
		assert.Equal(t, 1, f1.DestroyCount)
		assert.Equal(t, 1, f2.DestroyCount)

		_, err = manager.GetPlugin(logType, "default")
		assert.ErrorIs(t, err, ErrPluginNotFound)
	})

	t.Run("SetupRollbackOnFailure", func(t *testing.T) {
		manager := NewManager()
		okFactory := &MockFactory{PluginType: logType, ImplName: "oklogger"}
		failFactory := &MockFactory{PluginType: logType, ImplName: "faillogger", SetupErr: errors.New("setup failed")}
		manager.RegisterFactory(okFactory)
		manager.RegisterFactory(failFactory)

		// 直接调用内部函数，避免 map 迭代顺序导致测试不稳定。
		manager.mu.Lock()
		factories := manager.factories[logType]
		ins, err := manager.setupSinglePlugin(logType, "oklogger", RawConfig{"tag": "default"}, factories)
		assert.NoError(t, err)
		_, err = manager.setupSinglePlugin(logType, "faillogger", RawConfig{"tag": "failed"}, factories)
		assert.ErrorIs(t, err, ErrFactorySetup)
		targets := manager.rollbackPlugins([]initializedPlugin{ins})
		manager.mu.Unlock()

		err = destroyPlugins(targets)
		assert.NoError(t, err)
		assert.Equal(t, 1, okFactory.DestroyCount)

		_, err = manager.GetPlugin(logType, "default")
		assert.ErrorIs(t, err, ErrPluginNotFound)
	})
}
