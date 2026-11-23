// qqt-launcher-ctl is the native controller used by the WinForms launcher.
// It deliberately has no UI dependencies so process supervision remains
// testable and reliable even if the shell is closed or restarted.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"qqtang/internal/launcherapp"
	"qqtang/internal/server/networksetup"
)

type response struct {
	OK             bool               `json:"ok"`
	Message        string             `json:"message,omitempty"`
	Status         launcherapp.Status `json:"status"`
	IPCandidates   []string           `json:"ip_candidates,omitempty"`
	DiagnosticPath string             `json:"diagnostic_path,omitempty"`
}

// remoteConfig is the published bootstrap document. Only server_ip is
// required; mode and client_server_ip default to the local settings.
type remoteConfig struct {
	ServerIP       string              `json:"server_ip"`
	ClientServerIP string              `json:"client_server_ip"`
	Mode           string              `json:"mode"`
	Version        string              `json:"version"`
	PackageURL     string              `json:"package_url"`
	SHA256         string              `json:"sha256"`
	Ports          *networksetup.Ports `json:"ports"`
}

func configCachePath(root string) string {
	return filepath.Join(root, "runtime", "bootstrap-config.json")
}

func fetchRemoteConfig(root, url string, timeout time.Duration) (remoteConfig, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return remoteConfig{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return remoteConfig{}, fmt.Errorf("配置地址返回 %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return remoteConfig{}, err
	}
	var cfg remoteConfig
	body = bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf})
	if err = json.Unmarshal(body, &cfg); err != nil {
		// Plain-text bootstrap: one key=value per line, e.g.
		// server_ip=1.2.3.4
		// mode=remote-host
		cfg = parseTextConfig(body)
		if cfg.ServerIP == "" {
			return remoteConfig{}, fmt.Errorf("配置既不是 JSON 也没有 server_ip")
		}
	}
	cfg.ServerIP = strings.TrimSpace(cfg.ServerIP)
	cfg.ClientServerIP = strings.TrimSpace(cfg.ClientServerIP)
	cfg.Mode = strings.TrimSpace(cfg.Mode)
	if cfg.ServerIP == "" {
		return remoteConfig{}, fmt.Errorf("配置缺少 server_ip")
	}
	if err = os.MkdirAll(filepath.Dir(configCachePath(root)), 0o755); err == nil {
		_ = os.WriteFile(configCachePath(root), body, 0o644)
	}
	return cfg, nil
}

func readCachedConfig(root string) (remoteConfig, error) {
	body, err := os.ReadFile(configCachePath(root))
	if err != nil {
		return remoteConfig{}, err
	}
	var cfg remoteConfig
	if err = json.Unmarshal(body, &cfg); err != nil {
		return remoteConfig{}, err
	}
	if strings.TrimSpace(cfg.ServerIP) == "" {
		return remoteConfig{}, fmt.Errorf("缓存配置缺少 server_ip")
	}
	return cfg, nil
}

func parseTextConfig(body []byte) remoteConfig {
	var cfg remoteConfig
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			key, value, ok = strings.Cut(line, ":")
		}
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "server_ip", "serverip", "host", "ip":
			cfg.ServerIP = strings.TrimSpace(value)
		case "client_server_ip", "clientserverip":
			cfg.ClientServerIP = strings.TrimSpace(value)
		case "mode":
			cfg.Mode = strings.TrimSpace(value)
		}
	}
	return cfg
}

func main() {
	root := flag.String("root", "", "QQTang-Local root")
	action := flag.String("action", "status", "status, start-server, stop-server, start-client, stop-clients, or save-gm")
	mode := flag.String("mode", "", "local, lan-host, or remote-host")
	serverIP := flag.String("server-ip", "", "published server IPv4 address or DNS name")
	clientIP := flag.String("client-ip", "", "client target IPv4 address or DNS name")
	maximumFPS := flag.Int("max-fps", 144, "client frame-rate limit: 45, 60, 144, or 300")
	showFPS := flag.Bool("show-fps", false, "show the client's FPS counter")
	gmRemote := flag.Bool("gm-remote", false, "allow remote GM access")
	username := flag.String("username", "", "GM username")
	passwordStdin := flag.Bool("password-stdin", false, "read the GM password from stdin")
	configURL := flag.String("config-url", "", "HTTP(S) endpoint returning {\"server_ip\":\"...\"}")
	flag.Parse()

	if *root == "" {
		located, err := launcherapp.LocateRoot()
		if err != nil {
			fatal("", err)
		}
		*root = located
	}
	manager, err := launcherapp.New(*root)
	if err != nil {
		fatal(*root, err)
	}

	// Published bootstrap: the operator changes one JSON file behind a stable
	// URL, so moving the game server never requires redistributing launchers.
	if *configURL != "" && (*action == "start-server" || *action == "start-client" || *action == "resolve-config" || *action == "update") {
		remote, remoteErr := fetchRemoteConfig(*root, *configURL, 4*time.Second)
		if remoteErr != nil {
			manager.RecordDiagnostic("远程配置获取失败，使用本地缓存：%v", remoteErr)
			remote, remoteErr = readCachedConfig(*root)
		}
		if remoteErr != nil {
			fatal(*root, fmt.Errorf("远程配置不可用：%w", remoteErr))
		}
		*serverIP = remote.ServerIP
		if remote.ClientServerIP != "" {
			*clientIP = remote.ClientServerIP
		}
		if remote.Mode != "" {
			*mode = remote.Mode
		}
		if remote.Ports != nil {
			if saveErr := networksetup.SavePorts(filepath.Join(*root, "configs", "ports.json"), *remote.Ports); saveErr != nil {
				manager.RecordDiagnostic("写入端口配置失败：%v", saveErr)
			}
		}
	}

	var status launcherapp.Status
	var message string
	switch *action {
	case "status":
		status = manager.Status()
	case "resolve-config":
		status = manager.Status()
		message = "server_ip=" + *serverIP
	case "update":
		status = manager.Status()
		updated, updateMessage, updateErr := runUpdate(*root, *configURL)
		if updateErr != nil {
			err = updateErr
			break
		}
		message = updateMessage
		if updated {
			message += "；请重新启动启动器"
		}
	case "start-server":
		settings, loadErr := manager.LoadSettings()
		if loadErr != nil {
			settings = networksetup.Default()
		}
		if *mode != "" {
			settings.Mode = networksetup.Mode(*mode)
		}
		if *serverIP != "" {
			settings.ServerIP = strings.TrimSpace(*serverIP)
		}
		if *clientIP != "" {
			settings.ClientServerIP = strings.TrimSpace(*clientIP)
		} else {
			settings.ClientServerIP = settings.ServerIP
		}
		settings.GMRemote = *gmRemote
		status, err = manager.StartServer(settings)
		message = "服务端已启动"
	case "stop-server":
		status, err = manager.StopServer()
		message = "服务端已停止"
	case "start-client":
		target := strings.TrimSpace(*serverIP)
		if target == "" {
			target = "127.0.0.1"
		}
		status, err = manager.StartClient(target, *maximumFPS, *showFPS)
		message = "客户端已启动"
	case "stop-clients":
		status, err = manager.StopClients()
		message = "客户端已全部停止"
	case "save-gm":
		if !*passwordStdin {
			err = fmt.Errorf("save-gm requires -password-stdin")
			break
		}
		password, readErr := io.ReadAll(io.LimitReader(os.Stdin, 1024))
		if readErr != nil {
			err = fmt.Errorf("读取 GM 密码：%w", readErr)
			break
		}
		err = manager.SaveGMCredentials(*username, strings.TrimRight(string(password), "\r\n"))
		status = manager.Status()
		message = "远程 GM 密码已保存"
	case "check-client":
		status = manager.Status()
		missing := missingClientFiles(filepath.Join(*root, "runtime", "client-patched"))
		if len(missing) > 0 {
			err = fmt.Errorf("缺少客户端文件：%s", strings.Join(missing, ", "))
			break
		}
		message = "客户端文件完整"
	default:
		err = fmt.Errorf("unknown action %q", *action)
	}
	if err != nil {
		manager.RecordDiagnostic("控制器 action=%s 失败：%v", *action, err)
		fatal(*root, err)
	}
	emit(response{
		OK:             true,
		Message:        message,
		Status:         status,
		IPCandidates:   networksetup.PrivateIPv4Candidates(),
		DiagnosticPath: manager.DiagnosticPath(),
	})
}

func missingClientFiles(clientRoot string) []string {
	required := []string{"Client.exe", "Core.dll", "QQTDir.dll", "QQTModules.dll", "QQTSection.dll", filepath.Join("config", "GameCFG.ini")}
	missing := make([]string, 0, len(required))
	for _, name := range required {
		if _, err := os.Stat(filepath.Join(clientRoot, name)); err != nil {
			missing = append(missing, name)
		}
	}
	return missing
}

func fatal(root string, err error) {
	diagnostic := ""
	if root != "" {
		diagnostic = root + "\\runtime\\logs\\launcher-ui.log"
	}
	emit(response{OK: false, Message: err.Error(), DiagnosticPath: diagnostic})
	os.Exit(1)
}

func emit(value response) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}
