package plugin

// Type is the type of plugin supported by the system.
type Type string

const (
	// Metrics .
	Metrics = "metrics"
	// Tracer .
	Tracer = "tracer"
	// CSTransport
	CSTransport Type = "cstransport"
	// SSTransport
	SSTransport = "sstransport"
)

// Factory is the interface for plugin factories.
type Factory interface {
	// Type returns the plugin type.
	Type() Type
	// Name returns the name of the plugin implementation.
	Name() string
	// ConfigType returns an empty struct that represents the plugin's configuration.
	// This struct will be populated by the manager using mapstructure.
	ConfigType() any
	// Setup initializes a plugin instance based on the configuration.
	Setup(any) (Plugin, error)

	// Destroy is responsible for gracefully shutting down the plugin instance.
	// It should release any resources held by the plugin, such as network connections,
	// file handles, or background goroutines, to ensure a clean termination.
	Destroy(Plugin)
}

// Plugin is the marker interface for all plugins.
// Its FactoryName method returns the unique name of the factory that created it,
// which is useful for identifying the plugin's implementation at runtime.
type Plugin interface {
	FactoryName() string
}
