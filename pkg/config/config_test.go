package config

import (
	"path/filepath"
	"testing"
)

func TestValidateRejectsMissingAndDuplicateApps(t *testing.T) {
	tests := []struct {
		name string
		cfg  GopmConfig
	}{
		{
			name: "empty",
			cfg:  GopmConfig{},
		},
		{
			name: "missing name",
			cfg: GopmConfig{Apps: []AppConfig{
				{Command: "go"},
			}},
		},
		{
			name: "missing command",
			cfg: GopmConfig{Apps: []AppConfig{
				{Name: "api"},
			}},
		},
		{
			name: "duplicate name",
			cfg: GopmConfig{Apps: []AppConfig{
				{Name: "api", Command: "go"},
				{Name: "api", Command: "go"},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateAcceptsMinimalApp(t *testing.T) {
	cfg := GopmConfig{Apps: []AppConfig{
		{Name: "api", Command: "go"},
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestResolvePath(t *testing.T) {
	configPath := filepath.Join("tmp", "project", "gopm.yaml")
	got := ResolvePath(configPath, filepath.Join("logs", "app.log"))
	want := filepath.Join("tmp", "project", "logs", "app.log")
	if got != want {
		t.Fatalf("ResolvePath() = %q, want %q", got, want)
	}
}
