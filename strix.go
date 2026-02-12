// Package strix is the main package for the Strix application framework.
// It provides the core application structure and lifecycle management.
package strix

import (
	"github.com/linchenxuan/strix/log"
	"github.com/linchenxuan/strix/network/transport/sidecar"
	"github.com/linchenxuan/strix/network/transport/tcp"
	"github.com/linchenxuan/strix/plugin"
	"github.com/linchenxuan/strix/tracing"
)

// Strix is the core application struct. It acts as a container for all major
// framework components and dependencies, such as logging, plugin management, and tracing.
// It represents a single, cohesive application instance.
type Strix struct {
	Logger        log.Logger      // The application's primary logger instance.
	PluginManager *plugin.Manager // The manager for all registered plugins.
	Tracer        tracing.Tracer  // The application's tracer for distributed tracing.
}

// NewStrix creates and initializes a new Strix application instance.
// It sets up the default logger, plugin manager, and tracer, including setting
// the global instances for each to ensure easy access throughout the application.
func NewStrix() (*Strix, error) {
	// 1. Initialize the default logger.
	logCfg := &log.LogCfg{
		ConsoleAppender:   true,
		LogLevel:          log.DebugLevel,
		EnabledCallerInfo: true,
		CallerSkip:        1,
	}
	logger := log.NewLogger(logCfg)
	// Set this logger as the global default for convenient access via log.G().
	log.SetDefaultLogger(logger)

	// 2. Initialize the plugin manager.
	pluginManager := plugin.NewManager()
	pluginManager.RegisterFactory(tcp.NewFactory())
	pluginManager.RegisterFactory(sidecar.NewFactory())

	// 3. Initialize the tracer.
	tracer := tracing.NewTracer()
	// Set this tracer as the global default.
	tracing.SetGlobalTracer(tracer)

	// 4. Assemble the main Strix application instance.
	s := &Strix{
		Logger:        logger,
		PluginManager: pluginManager,
		Tracer:        tracer,
	}

	logger.Info().Msg("Strix application initialized")
	return s, nil
}

// Stop gracefully shuts down the Strix application. It ensures that all
// components, such as the tracer, are properly closed to prevent resource leaks.
func (s *Strix) Stop() {
	s.Logger.Info().Msg("Strix application shutting down")
	// Close the tracer to flush any buffered spans.
	s.Tracer.Close()
}
