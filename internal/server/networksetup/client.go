package networksetup

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type clientEndpointFile struct {
	name string
	keys []string
}

var clientEndpointFiles = []clientEndpointFile{
	{name: "DirCfg.ini", keys: []string{"DirIP"}},
	{name: "p2psvrInfo.ini", keys: []string{"tcpip", "udpip", "stunip"}},
	{name: "caserver.ini", keys: []string{"ip", "ip2"}},
	{name: "webserver.ini", keys: []string{"ip", "ip2"}},
}

// ApplyClientTarget updates only the explicit server-address keys used by the
// prepared client. Original client files under source/ are never touched.
func ApplyClientTarget(clientRoot, serverIP string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return applyClientTarget(ctx, clientRoot, serverIP, net.DefaultResolver.LookupNetIP)
}

// ApplyClientTargetWithPorts writes both the resolved IPv4 and the configured
// ports into the patched client INI files. This keeps one readable
// configs/ports.json authoritative for server, launcher and client.
func ApplyClientTargetWithPorts(clientRoot, serverIP string, ports Ports) error {
	if err := ApplyClientTarget(clientRoot, serverIP); err != nil {
		return err
	}
	return applyClientPorts(clientRoot, ports.withDefaults())
}

func applyClientPorts(clientRoot string, ports Ports) error {
	dirPath := filepath.Join(clientRoot, "DirCfg.ini")
	if err := rewriteDirCfgPorts(dirPath, ports); err != nil {
		return err
	}
	if err := rewriteKeyPorts(filepath.Join(clientRoot, "p2psvrInfo.ini"), map[string]int{
		"tcpport": ports.TLS, "udpport": ports.UDP, "stunport": ports.UDP,
	}); err != nil {
		return err
	}
	if err := rewriteKeyPorts(filepath.Join(clientRoot, "caserver.ini"), map[string]int{
		"port": ports.Directory, "port2": ports.Secondary,
	}); err != nil {
		return err
	}
	if err := rewriteKeyPorts(filepath.Join(clientRoot, "webserver.ini"), map[string]int{
		"port": ports.Directory, "port2": ports.Directory,
	}); err != nil {
		return err
	}
	return nil
}

func rewriteDirCfgPorts(path string, ports Ports) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	byIndex := map[int]int{0: ports.Directory, 1: ports.Game, 2: ports.TLS, 3: ports.Directory, 4: ports.Game, 5: ports.TLS}
	section := -1
	lines := strings.Split(string(data), "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[Dir") && strings.HasSuffix(trimmed, "]") {
			fmt.Sscanf(trimmed, "[Dir%d]", &section)
			continue
		}
		if section >= 0 && strings.HasPrefix(strings.ToLower(trimmed), "dirport=") {
			if port, ok := byIndex[section]; ok {
				lines[index] = "DirPort=" + strconv.Itoa(port)
			}
		}
	}
	return replaceFile(path, []byte(strings.Join(lines, "\n")))
}

func rewriteKeyPorts(path string, values map[string]int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		key, _, found := strings.Cut(trimmed, "=")
		if !found {
			continue
		}
		if port, ok := values[strings.ToLower(strings.TrimSpace(key))]; ok {
			lines[index] = key + "=" + strconv.Itoa(port)
		}
	}
	return replaceFile(path, []byte(strings.Join(lines, "\n")))
}

func applyClientTarget(ctx context.Context, clientRoot, serverHost string, lookup func(context.Context, string, string) ([]netip.Addr, error)) error {
	address, err := resolveEndpointIPv4(ctx, serverHost, false, lookup)
	if err != nil {
		return fmt.Errorf("resolve client server address %q: %w", serverHost, err)
	}
	host := address.String()
	type endpointUpdate struct {
		name     string
		path     string
		original []byte
		updated  []byte
	}
	updates := make([]endpointUpdate, 0, len(clientEndpointFiles))
	for _, endpointFile := range clientEndpointFiles {
		path := filepath.Join(clientRoot, endpointFile.name)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read client endpoint file %s: %w", endpointFile.name, readErr)
		}
		updated := data
		for _, key := range endpointFile.keys {
			pattern := regexp.MustCompile(`(?mi)^` + regexp.QuoteMeta(key) + `=[^\r\n]*`)
			if !pattern.Match(updated) {
				return fmt.Errorf("client endpoint file %s has no %s key", endpointFile.name, key)
			}
			updated = pattern.ReplaceAll(updated, []byte(key+"="+host))
		}
		updates = append(updates, endpointUpdate{
			name: endpointFile.name, path: path,
			original: append([]byte(nil), data...), updated: append([]byte(nil), updated...),
		})
	}
	for index, update := range updates {
		if err := replaceFile(update.path, update.updated); err != nil {
			rollbackErrors := make([]string, 0, index)
			for rollbackIndex := index - 1; rollbackIndex >= 0; rollbackIndex-- {
				previous := updates[rollbackIndex]
				if rollbackErr := replaceFile(previous.path, previous.original); rollbackErr != nil {
					rollbackErrors = append(rollbackErrors, fmt.Sprintf("restore %s: %v", previous.name, rollbackErr))
				}
			}
			if len(rollbackErrors) != 0 {
				return fmt.Errorf("update client endpoint file %s: %w; rollback failed: %s", update.name, err, strings.Join(rollbackErrors, "; "))
			}
			return fmt.Errorf("update client endpoint file %s: %w", update.name, err)
		}
	}
	return nil
}

func replaceFile(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".endpoint-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
