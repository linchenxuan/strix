package strix

import (
	"testing"

	"github.com/linchenxuan/strix/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewStrix verifies that calling NewStrix
// successfully creates a default Strix instance.
func TestNewStrix(t *testing.T) {
	app, err := NewStrix()
	require.NoError(t, err)
	require.NotNil(t, app)

	assert.NotNil(t, app.Logger, "Default logger should not be nil")
	assert.NotNil(t, app.PluginManager, "Default plugin manager should not be nil")
	assert.NotNil(t, app.Tracer, "Default tracer should not be nil")
}

// TestStrixStop verifies that the Stop method runs without panicking.
func TestStrixStop(t *testing.T) {
	app, err := NewStrix()
	require.NoError(t, err)
	require.NotNil(t, app)

	// Just ensure Stop() doesn't panic
	assert.NotPanics(t, func() {
		app.Stop()
	})
}

// TestBuiltInTCPFactoryRegistration verifies that NewStrix wires built-in transport
// factories into the plugin manager and that TCP plugin setup works with config decoding.
func TestBuiltInTCPFactoryRegistration(t *testing.T) {
	app, err := NewStrix()
	require.NoError(t, err)
	require.NotNil(t, app)

	conf := map[string]any{
		string(plugin.CSTransport): map[string]any{
			"tcp_transport": map[string]any{
				"tag":             plugin.DefaultInsName,
				"idleTimeout":     30,
				"crypt":           0,
				"addr":            "127.0.0.1:0",
				"connType":        "server",
				"frameMetaKey":    "default",
				"sendChannelSize": 64,
				"maxBufferSize":   4096,
			},
		},
	}

	err = app.PluginManager.SetupPlugins(conf)
	require.NoError(t, err)

	p, err := app.PluginManager.GetDefaultPlugin(plugin.CSTransport)
	require.NoError(t, err)
	require.NotNil(t, p)
}
