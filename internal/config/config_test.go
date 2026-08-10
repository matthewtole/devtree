package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse_valid(t *testing.T) {
	src := `
[[service]]
name    = "backend"
repo    = "/abs/path/backend"
command = "npm run dev"

[[service]]
name    = "web"
repo    = "/abs/path/web"
command = "npm run dev"
`
	cfg, err := Parse([]byte(src), "test.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := len(cfg.Services), 2; got != want {
		t.Fatalf("len(services) = %d, want %d", got, want)
	}
	if got, want := cfg.Services[0].Name, "backend"; got != want {
		t.Errorf("services[0].Name = %q, want %q", got, want)
	}
	if got, want := cfg.Services[1].Command, "npm run dev"; got != want {
		t.Errorf("services[1].Command = %q, want %q", got, want)
	}
}

func TestParse_errors(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantSub string
	}{
		{
			name:    "empty config",
			src:     ``,
			wantSub: "no services defined",
		},
		{
			name: "missing name",
			src: `
[[service]]
repo    = "/abs/path"
command = "x"
`,
			wantSub: "name is required",
		},
		{
			name: "duplicate name",
			src: `
[[service]]
name    = "web"
repo    = "/a"
command = "x"

[[service]]
name    = "web"
repo    = "/b"
command = "y"
`,
			wantSub: `duplicate name "web"`,
		},
		{
			name: "relative repo",
			src: `
[[service]]
name    = "web"
repo    = "relative/path"
command = "x"
`,
			wantSub: "must be an absolute path",
		},
		{
			name: "missing command",
			src: `
[[service]]
name = "web"
repo = "/abs"
`,
			wantSub: "command is required",
		},
		{
			name: "unknown key",
			src: `
[[service]]
name      = "web"
repo      = "/abs"
command   = "x"
mystery   = "field"
`,
			wantSub: "unknown keys",
		},
		{
			name:    "malformed toml",
			src:     `not = "valid" = toml`,
			wantSub: "parse",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.src), "test.toml")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestLoad_missingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.toml")
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "config not found") {
		t.Errorf("error %q does not mention missing file", err.Error())
	}
}

func TestLoadServices_missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.toml")
	svcs, err := LoadServices(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svcs != nil {
		t.Errorf("expected nil services for missing file, got %v", svcs)
	}
}

func TestLoadServices_valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	src := `
[[service]]
name    = "api"
repo    = "/repos/api"
command = "go run ."
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	svcs, err := LoadServices(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := len(svcs), 1; got != want {
		t.Fatalf("len = %d, want %d", got, want)
	}
	if svcs[0].Name != "api" {
		t.Errorf("Name = %q, want %q", svcs[0].Name, "api")
	}
}

func TestAppendService_createsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	svc := Service{Name: "web", Repo: "/repos/web", Command: "npm start"}
	if err := AppendService(path, svc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svcs, err := LoadServices(path)
	if err != nil {
		t.Fatalf("load after append: %v", err)
	}
	if got, want := len(svcs), 1; got != want {
		t.Fatalf("len = %d, want %d", got, want)
	}
	if svcs[0].Name != "web" || svcs[0].Repo != "/repos/web" || svcs[0].Command != "npm start" {
		t.Errorf("unexpected service: %+v", svcs[0])
	}
}

func TestAppendService_appendsToExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	src := `[[service]]
name    = "api"
repo    = "/repos/api"
command = "go run ."
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := Service{Name: "web", Repo: "/repos/web", Command: "npm start"}
	if err := AppendService(path, svc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	svcs, err := LoadServices(path)
	if err != nil {
		t.Fatalf("load after append: %v", err)
	}
	if got, want := len(svcs), 2; got != want {
		t.Fatalf("len = %d, want %d", got, want)
	}
	if svcs[1].Name != "web" {
		t.Errorf("second service Name = %q, want %q", svcs[1].Name, "web")
	}
}

func TestDefaultPath_xdg(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/xdg")
	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "/custom/xdg/devtree/config.toml"; got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestDefaultPath_homeFallback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/tmp/fakehome")
	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "/tmp/fakehome/.config/devtree/config.toml"; got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}
