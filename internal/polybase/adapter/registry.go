package adapter

import "fmt"

// Constructor is a factory function for an Adapter.
type Constructor func() Adapter

var registry = map[string]Constructor{}

// Register registers a constructor for the named engine.
// Called from each adapter package's init().
func Register(engine string, fn Constructor) {
	registry[engine] = fn
}

// New returns a fresh (unconnected) Adapter for the given engine.
func New(engine string) (Adapter, error) {
	fn, ok := registry[engine]
	if !ok {
		return nil, fmt.Errorf("adapter: unknown engine %q (supported: postgres, mongodb)", engine)
	}
	return fn(), nil
}

// Engines returns the list of registered engine names.
func Engines() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}
