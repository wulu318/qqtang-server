package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTextConfig(t *testing.T) {
	cfg := parseTextConfig([]byte("# comment\nserver_ip=203.0.113.9\nmode=remote-host\n"))
	if cfg.ServerIP != "203.0.113.9" || cfg.Mode != "remote-host" {
		t.Fatalf("unexpected config %+v", cfg)
	}
}

func TestReplaceTreeProtectsSaves(t *testing.T) {
	source := t.TempDir()
	root := t.TempDir()
	mustWrite := func(base, name, content string) {
		path := filepath.Join(base, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(source, "runtime/data/qqtang.sqlite", "new")
	mustWrite(source, "runtime/bin/app.exe", "binary")
	mustWrite(root, "runtime/data/qqtang.sqlite", "old-save")
	if err := replaceTree(source, root); err != nil {
		t.Fatal(err)
	}
	save, _ := os.ReadFile(filepath.Join(root, "runtime/data/qqtang.sqlite"))
	if string(save) != "old-save" {
		t.Fatalf("save was overwritten: %q", save)
	}
	binary, _ := os.ReadFile(filepath.Join(root, "runtime/bin/app.exe"))
	if string(binary) != "binary" {
		t.Fatalf("binary was not replaced: %q", binary)
	}
}
