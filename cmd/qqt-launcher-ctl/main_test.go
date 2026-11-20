package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFetchRemoteConfigParsesAndCaches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"server_ip":"203.0.113.9","mode":"remote-host"}`))
	}))
	defer server.Close()

	root := t.TempDir()
	cfg, err := fetchRemoteConfig(root, server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerIP != "203.0.113.9" || cfg.Mode != "remote-host" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if _, err = os.Stat(filepath.Join(root, "runtime", "bootstrap-config.json")); err != nil {
		t.Fatalf("cache not written: %v", err)
	}
	cached, err := readCachedConfig(root)
	if err != nil || cached.ServerIP != cfg.ServerIP {
		t.Fatalf("cache read = %+v err=%v", cached, err)
	}
}

func TestFetchRemoteConfigRejectsMissingServerIP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"mode":"remote-host"}`))
	}))
	defer server.Close()
	if _, err := fetchRemoteConfig(t.TempDir(), server.URL, time.Second); err == nil {
		t.Fatal("missing server_ip was accepted")
	}
}
