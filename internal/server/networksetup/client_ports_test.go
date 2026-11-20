package networksetup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyClientPortsRewritesAllEndpointFiles(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("DirCfg.ini", "[public]\n[Dir0]\nDirPort=18080\n[Dir1]\nDirPort=18000\n[Dir2]\nDirPort=18443\n[Dir3]\nDirPort=18080\n[Dir4]\nDirPort=18000\n[Dir5]\nDirPort=18443\n")
	write("p2psvrInfo.ini", "tcpport=18443\nudpport=18000\nstunport=18000\n")
	write("caserver.ini", "port=18080\nport2=17000\n")
	write("webserver.ini", "port=18080\nport2=18080\n")

	ports := Ports{Directory: 28080, Game: 28000, Shop: 28001, Secondary: 27000, TLS: 28443, GM: 28100, UDP: 28000}
	if err := applyClientPorts(root, ports); err != nil {
		t.Fatal(err)
	}
	dir, _ := os.ReadFile(filepath.Join(root, "DirCfg.ini"))
	text := string(dir)
	for _, want := range []string{"DirPort=28080", "DirPort=28000", "DirPort=28443"} {
		if !strings.Contains(text, want) {
			t.Fatalf("DirCfg.ini missing %s:\n%s", want, text)
		}
	}
	p2p, _ := os.ReadFile(filepath.Join(root, "p2psvrInfo.ini"))
	if !strings.Contains(string(p2p), "udpport=28000") || !strings.Contains(string(p2p), "tcpport=28443") {
		t.Fatalf("p2psvrInfo.ini not rewritten:\n%s", p2p)
	}
	ca, _ := os.ReadFile(filepath.Join(root, "caserver.ini"))
	if !strings.Contains(string(ca), "port=28080") || !strings.Contains(string(ca), "port2=27000") {
		t.Fatalf("caserver.ini not rewritten:\n%s", ca)
	}
}
