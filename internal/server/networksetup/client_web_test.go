package networksetup

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPatchClientWebURLsKeepsByteLengthAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "res", "uiRes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := append([]byte{0x3b, 0xf2, 0x0d, 0x0a}, []byte("prefix"+embeddedFamilyRankURL+"suffix")...)
	path := filepath.Join(dir, "uiMain.pyc")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := patchClientWebURLs(root, "203.0.113.5", 18080); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(original) {
		t.Fatalf("length changed: %d -> %d", len(original), len(got))
	}
	if bytes.Contains(got, []byte("app.qqtang.qq.com")) {
		t.Fatal("old URL still present")
	}
	if !bytes.Contains(got, []byte("http%3A%2F%2F203.0.113.5%3A18080%2Fleaderboard")) {
		t.Fatalf("new URL missing: %q", string(got))
	}
	if err := patchClientWebURLs(root, "203.0.113.5", 18080); err != nil {
		t.Fatal(err)
	}
}
