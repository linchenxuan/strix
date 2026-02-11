// Package pool provides a wrapper around sync.Pool with added metrics.
package pool

import (
	"sync"

	"github.com/linchenxuan/strix/metrics"
)

// Pool is a wrapper around sync.Pool that collects metrics on object creation.
type Pool struct {
	Name string     // Name is the name of the pool, used as a dimension in metrics.
	Pool *sync.Pool // Pool is the underlying sync.Pool instance.
}

// NewPool creates a new instrumented pool.
// The 'name' is used for metrics reporting.
// The 'newFunc' is the function called to create a new item when the pool is empty.
func NewPool(name string, newFunc func() any) *Pool {
	p := &Pool{
		Name: name,
	}

	p.Pool = &sync.Pool{
		New: func() any {
			// Increment a counter every time a new object is created because the pool was empty.
			metrics.IncrCounterWithDimGroup(metrics.NamePoolCreateTotal, metrics.GroupStrix, 1, metrics.Dimension{
				metrics.DimPoolName: name,
			})
			return newFunc()
		},
	}
	return p
}

// Put adds x back to the pool for reuse.
func (p *Pool) Put(x any) {
	p.Pool.Put(x)
}

// Get retrieves an item from the pool.
// If the pool is empty, a new item is created using the 'newFunc' provided to NewPool.
func (p *Pool) Get() any {
	return p.Pool.Get()
}
