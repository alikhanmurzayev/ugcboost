package closer

import (
	"context"
	"sync"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

// Func is a shutdown function that may return an error.
type Func func(ctx context.Context) error

// Closer manages ordered shutdown of resources.
// Resources are closed in LIFO order (last added = first closed).
type Closer struct {
	mu     sync.Mutex
	funcs  []namedFunc
	logger logger.Logger
}

type namedFunc struct {
	name string
	fn   Func
}

// New creates a new Closer.
func New(log logger.Logger) *Closer {
	return &Closer{logger: log}
}

// Add registers a shutdown function with a descriptive name.
func (c *Closer) Add(name string, fn Func) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.funcs = append(c.funcs, namedFunc{name: name, fn: fn})
}

// Close executes all registered functions in reverse order (LIFO).
// All functions are called even if some return errors.
func (c *Closer) Close(ctx context.Context) error {
	c.mu.Lock()
	funcs := make([]namedFunc, len(c.funcs))
	copy(funcs, c.funcs)
	c.mu.Unlock()

	var firstErr error
	for i := len(funcs) - 1; i >= 0; i-- {
		nf := funcs[i]
		c.logger.Info(ctx, "shutting down", "resource", nf.name)
		if err := nf.fn(ctx); err != nil {
			c.logger.Error(ctx, "shutdown error", "resource", nf.name, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
