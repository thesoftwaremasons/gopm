package adapter

import "fmt"

type Constructor func() Adapter

var registry = map[string]Constructor{}

func Register(engine string, fn Constructor) {
	registry[engine] = fn
}

func New(engine string) (Adapter, error) {
	fn, ok := registry[engine]
	if !ok {
		return nil, fmt.Errorf("adapter: unknown engine %q", engine)
	}
	return fn(), nil
}

func Engines() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}
