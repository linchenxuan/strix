package metrics

import "sync"

var (
	_Reporters       []Reporter
	setReportersOnce sync.Once
)

// Reporter 定义指标上报实现接口。
// 不同 reporter 可将指标发送到不同后端，
// 例如 Prometheus、StatsD、InfluxDB 等。
type Reporter interface {
	Report(r Record)
}

// SetReporters 设置全局 reporter 列表。
// 指标更新时会同时上报到这些 reporter。
func SetReporters(reports []Reporter) {
	called := false
	setReportersOnce.Do(func() {
		called = true
		_Reporters = append([]Reporter(nil), reports...)
	})
	if !called {
		panic("metrics reporters have already been initialized")
	}
}
