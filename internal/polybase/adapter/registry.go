package adapter

import (
	"context"
	"fmt"
	"sync"
)

// FactoryFunc creates a new, unconnected Adapter.
type FactoryFunc func() Adapter

var (
	mu        sync.RWMutex
	factories = map[string]FactoryFunc{}
)

// Register registers a factory for the given engine name.
func Register(engine string, fn FactoryFunc) {
	mu.Lock()
	defer mu.Unlock()
	factories[engine] = fn
}

// New creates a new adapter for the given engine.
func New(engine string) (Adapter, error) {
	mu.RLock()
	fn, ok := factories[engine]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown engine: %q", engine)
	}
	return fn(), nil
}

// Pool manages active connections keyed by connection ID.
type Pool struct {
	mu      sync.RWMutex
	active  map[string]Adapter
}

// NewPool creates a new connection pool.
func NewPool() *Pool {
	return &Pool{active: make(map[string]Adapter)}
}

// GetOrConnect returns an existing adapter or creates and connects a new one.
func (p *Pool) GetOrConnect(ctx context.Context, cfg ConnectionConfig) (Adapter, error) {
	p.mu.RLock()
	a, ok := p.active[cfg.ID]
	p.mu.RUnlock()
	if ok {
		return a, nil
	}

	a, err := New(cfg.Engine)
	if err != nil {
		return nil, err
	}
	if err := a.Connect(ctx, cfg); err != nil {
		return nil, err
	}

	p.mu.Lock()
	p.active[cfg.ID] = a
	p.mu.Unlock()
	return a, nil
}

// Remove closes and removes an adapter from the pool.
func (p *Pool) Remove(id string) {
	p.mu.Lock()
	a, ok := p.active[id]
	if ok {
		delete(p.active, id)
	}
	p.mu.Unlock()
	if ok {
		_ = a.Close()
	}
}

// CloseAll closes all active adapters.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, a := range p.active {
		_ = a.Close()
		delete(p.active, id)
	}
}
