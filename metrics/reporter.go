package metrics

var _Reporters []Reporter

// Reporter defines the interface for metric reporting implementations.
// Different reporters can be used to send metrics to various backends
// such as Prometheus, StatsD, InfluxDB, etc.
type Reporter interface {
	Report(r Record)
}
