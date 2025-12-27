package strix

import (
	"github.com/linchenxuan/strix/log"
	"github.com/linchenxuan/strix/plugin"
	"github.com/linchenxuan/strix/tracing"
)

// Strix is the core application struct, holding all major framework components and dependencies.
type Strix struct {
	Logger        log.Logger
	PluginManager *plugin.Manager
	Tracer        tracing.Tracer
}

// New creates a new Strix application instance with default configurations.
// It initializes the logger, plugin manager, and tracer.
func NewStrix() (*Strix, error) {
	// 1. Initialize Logger
	logCfg := &log.LogCfg{
		ConsoleAppender:   true,
		LogLevel:          log.DebugLevel,
		EnabledCallerInfo: true,
		CallerSkip:        1,
	}
	logger := log.NewLogger(logCfg)

	// Set the created logger as the global default for convenient access
	log.SetDefaultLogger(logger)

	// 2. Initialize Plugin Manager
	pluginManager := plugin.NewManager()

	// 3. Initialize Tracer
	tracer := tracing.NewTracer()
	tracing.SetGlobalTracer(tracer)

	// 4. Assemble Strix instance
	s := &Strix{
		Logger:        logger,
		PluginManager: pluginManager,
		Tracer:        tracer,
	}

	logger.Info().Msg("Strix application initialized")
	return s, nil
}

// Stop gracefully shuts down the Strix application, closing all components.
func (s *Strix) Stop() {
	s.Logger.Info().Msg("Strix application shutting down")
	s.Tracer.Close()
}
