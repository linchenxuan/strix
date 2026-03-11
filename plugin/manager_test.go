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

func TestGlobalAPI_SetupAndGetDefaultPlugin(t *testing.T) {
	resetManagerForTest()

	RegisterFactory(&MockFactory{PluginType: logType, ImplName: "mocklogger"})

	err := SetupPlugins(Config{
		logType: TypeConfig{
			"mocklogger": RawConfig{"tag": "default"},
		},
	})
	assert.NoError(t, err)

	p, err := GetDefaultPlugin(logType)
	assert.NoError(t, err)
	assert.IsType(t, &MockPlugin{}, p)
}

func TestManager(t *testing.T) {
	factory := &MockFactory{PluginType: logType, ImplName: "mocklogger"}

	t.Run("RegisterFactory", func(t *testing.T) {
		resetManagerForTest()

		RegisterFactory(factory)
		assert.NotNil(t, defaultManager.factories[logType])
		assert.Equal(t, factory, defaultManager.factories[logType]["mocklogger"])
	})

	t.Run("SetupAndGetPlugins", func(t *testing.T) {
		resetManagerForTest()

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
		RegisterFactory(anotherFactory)
		RegisterFactory(factory)

		err := SetupPlugins(pluginConf)
		assert.NoError(t, err)

		p, err := GetPlugin(logType, "default")
		assert.NoError(t, err)
		assert.NotNil(t, p)
		assert.IsType(t, &MockPlugin{}, p)

		// 这里验证默认实例与按 tag 获取到的是同一个对象。
		dp, err := GetDefaultPlugin(logType)
		assert.NoError(t, err)
		assert.IsType(t, &MockPlugin{}, dp)
		assert.Equal(t, p, dp)

		np, err := GetPlugin(logType, "anotherlogger")
		assert.NoError(t, err)
		assert.NotNil(t, np)
	})

	t.Run("ErrorOnDuplicateTag", func(t *testing.T) {
		resetManagerForTest()

		f1 := &MockFactory{PluginType: logType, ImplName: "logger1"}
		f2 := &MockFactory{PluginType: logType, ImplName: "logger2"}
		RegisterFactory(f1)
		RegisterFactory(f2)

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

		err := SetupPlugins(pluginConf)
		assert.ErrorIs(t, err, ErrDuplicatePlugin)
		// 第二个插件 setup 成功但注册失败，应被即时销毁，避免泄漏。
		assert.Equal(t, 1, f2.DestroyCount)
		_, getErr := GetPlugin(logType, "default")
		assert.ErrorIs(t, getErr, ErrPluginNotFound)
	})

	t.Run("ErrorOnMissingFactory", func(t *testing.T) {
		resetManagerForTest()

		RegisterFactory(&MockFactory{PluginType: logType, ImplName: "reallogger"})

		pluginConf := Config{
			logType: TypeConfig{
				"nonexistent": RawConfig{},
			},
		}
		err := SetupPlugins(pluginConf)
		assert.ErrorIs(t, err, ErrPluginNotFound)
	})

	t.Run("TestConfigDecoding", func(t *testing.T) {
		resetManagerForTest()

		mockConfigFactory := &MockFactory{PluginType: "test", ImplName: "mockconfigfactory"}
		RegisterFactory(mockConfigFactory)

		t.Run("SuccessfulDecoding", func(t *testing.T) {
			resetManagerForTest()
			RegisterFactory(mockConfigFactory)

			pluginConf := Config{
				Type("test"): TypeConfig{
					"mockconfigfactory": RawConfig{
						"Level":  "info",
						"Output": "stdout",
					},
				},
			}
			err := SetupPlugins(pluginConf)
			assert.NoError(t, err)
		})

		t.Run("FailedDecoding_InvalidType", func(t *testing.T) {
			resetManagerForTest()
			RegisterFactory(mockConfigFactory)

			pluginConf := Config{
				Type("test"): TypeConfig{
					"mockconfigfactory": RawConfig{
						"Level":  123, // string 字段给了 int，触发解码错误。
						"Output": "stdout",
					},
				},
			}
			err := SetupPlugins(pluginConf)
			assert.Error(t, err)
			assert.ErrorIs(t, err, ErrConfigDecode)
		})

		t.Run("FailedConfigType_Nil", func(t *testing.T) {
			resetManagerForTest()
			RegisterFactory(&NilConfigFactory{PluginType: "test", ImplName: "nilconfigfactory"})

			pluginConf := Config{
				Type("test"): TypeConfig{
					"nilconfigfactory": RawConfig{},
				},
			}
			err := SetupPlugins(pluginConf)
			assert.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidFactoryConfig)
		})
	})

	t.Run("DestroyPlugins", func(t *testing.T) {
		resetManagerForTest()
		f1 := &MockFactory{PluginType: logType, ImplName: "logger1"}
		f2 := &MockFactory{PluginType: logType, ImplName: "logger2"}
		RegisterFactory(f1)
		RegisterFactory(f2)

		pluginConf := Config{
			logType: TypeConfig{
				"logger1": RawConfig{"tag": "default"},
				"logger2": RawConfig{"tag": "logger2"},
			},
		}
		err := SetupPlugins(pluginConf)
		assert.NoError(t, err)

		err = Destroy()
		assert.NoError(t, err)
		assert.Equal(t, 1, f1.DestroyCount)
		assert.Equal(t, 1, f2.DestroyCount)

		_, err = GetPlugin(logType, "default")
		assert.ErrorIs(t, err, ErrPluginNotFound)
	})

	t.Run("SetupRollbackOnFailure", func(t *testing.T) {
		resetManagerForTest()
		okFactory := &MockFactory{PluginType: logType, ImplName: "oklogger"}
		failFactory := &MockFactory{PluginType: logType, ImplName: "faillogger", SetupErr: errors.New("setup failed")}
		RegisterFactory(okFactory)
		RegisterFactory(failFactory)

		// 直接调用内部函数，避免 map 迭代顺序导致测试不稳定。
		defaultManager.mu.Lock()
		factories := defaultManager.factories[logType]
		ins, err := defaultManager.setupSinglePlugin(logType, "oklogger", RawConfig{"tag": "default"}, factories)
		assert.NoError(t, err)
		_, err = defaultManager.setupSinglePlugin(logType, "faillogger", RawConfig{"tag": "failed"}, factories)
		assert.ErrorIs(t, err, ErrFactorySetup)
		targets := defaultManager.rollbackPlugins([]initializedPlugin{ins})
		defaultManager.mu.Unlock()

		err = destroyPlugins(targets)
		assert.NoError(t, err)
		assert.Equal(t, 1, okFactory.DestroyCount)

		_, err = GetPlugin(logType, "default")
		assert.ErrorIs(t, err, ErrPluginNotFound)
	})
}
