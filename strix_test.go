package strix

import (
	"testing"

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