package supervisor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thesoftwaremasons/gopm/pkg/config"
)

func TestParseEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("A=one\nB=\"two\"\n# ignored\nC='three'\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := parseEnvFile(path)
	if err != nil {
		t.Fatalf("parseEnvFile() error = %v", err)
	}
	if got["A"] != "one" || got["B"] != "two" || got["C"] != "three" {
		t.Fatalf("parseEnvFile() = %#v", got)
	}
}

func TestBuildCmdEnvPrecedence(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("A=file\nB=file\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s := New(nil)
	mp := newManagedProcess(1, config.AppConfig{
		Name:    "api",
		Command: "go",
		Cwd:     dir,
		EnvFile: ".env",
		Env: map[string]string{
			"B": "inline",
		},
	})

	cmd := s.buildCmd(mp)
	env := strings.Join(cmd.Env, "\n")
	if !strings.Contains(env, "A=file") {
		t.Fatalf("env missing A=file: %q", env)
	}
	if !strings.Contains(env, "B=inline") {
		t.Fatalf("env missing B=inline: %q", env)
	}
	if strings.Contains(env, "B=file") {
		t.Fatalf("env should not include overridden B=file: %q", env)
	}
}
