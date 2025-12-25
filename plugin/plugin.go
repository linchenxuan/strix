package plugin

// Type is the type of plugin supported by the system.
type Type string

const (
	// Metrics .
	Metrics = "metrics"
	// Tracer .
	Tracer = "tracer"
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

	Destroy(Plugin)
}

type Plugin interface {
	FactoryName() string
}
