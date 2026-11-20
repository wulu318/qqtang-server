package launcherapp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"qqtang/internal/server/gm"
	"qqtang/internal/server/networksetup"
)

const stateSchemaVersion = 5

const serverReadyTimeout = 30 * time.Second

type Manager struct {
	root string
	mu   sync.Mutex
}

type Status struct {
	ServerRunning bool                  `json:"server_running"`
	ServerPID     int                   `json:"server_pid"`
	ClientCount   int                   `json:"client_count"`
	Settings      networksetup.Settings `json:"settings"`
	GMURL         string                `json:"gm_url"`
}

type ProcessEntry struct {
	PID          int    `json:"pid"`
	Path         string `json:"path"`
	Instance     int    `json:"instance,omitempty"`
	ClientPID    int    `json:"client_pid,omitempty"`
	ServerID     uint32 `json:"server_id,omitempty"`
	StartTimeUTC string `json:"start_time_utc,omitempty"`
	Ready        string `json:"ready,omitempty"`
	Result       string `json:"result,omitempty"`
	LaunchResult string `json:"launch_result,omitempty"`
}

type RunState struct {
	SchemaVersion     int                   `json:"schema_version"`
	StartedUTC        string                `json:"started_utc,omitempty"`
	Config            string                `json:"config,omitempty"`
	DirectoryPacket   string                `json:"directory_packet,omitempty"`
	Network           networksetup.Settings `json:"network"`
	Server            *ProcessEntry         `json:"server,omitempty"`
	ClientCount       int                   `json:"client_count"`
	Clients           []ProcessEntry        `json:"clients"`
	LoginHelpers      []ProcessEntry        `json:"login_helpers"`
	ShopServerHelpers []ProcessEntry        `json:"shop_server_helpers"`
	Client            *ProcessEntry         `json:"client,omitempty"`
	LoginHelper       *ProcessEntry         `json:"login_helper,omitempty"`
}

func New(root string) (*Manager, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("瑙ｆ瀽鍚姩鐩綍：%w", err)
	}
	if _, err := os.Stat(filepath.Join(absolute, "configs", "server-directory-local-ui.json")); err != nil {
		return nil, fmt.Errorf("鍚姩鐩綍涓嶅畬鏁达紙缂哄皯 configs\\server-directory-local-ui.json锛夛細%s", absolute)
	}
	return &Manager{root: absolute}, nil
}

func LocateRoot() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	candidates := []string{filepath.Dir(executable), filepath.Clean(filepath.Join(filepath.Dir(executable), "..", ".."))}
	if current, currentErr := os.Getwd(); currentErr == nil {
		candidates = append(candidates, current)
	}
	for _, candidate := range candidates {
		if _, statErr := os.Stat(filepath.Join(candidate, "configs", "server-directory-local-ui.json")); statErr == nil {
			return filepath.Abs(candidate)
		}
	}
	return "", errors.New("鎵句笉鍒板畬鏁寸殑 QQTang-Local 鐩綍锛岃鍏堝畬鏁磋В鍘嬪彂甯冨寘")
}

func (manager *Manager) DiagnosticPath() string { return manager.paths().launcherLog }

// RecordDiagnostic persists controller failures even when an operation never
// reaches its normal success logging. The WinForms shell must never point at
// a diagnostic path that was not actually written.
func (manager *Manager) RecordDiagnostic(format string, values ...any) {
	if manager == nil {
		return
	}
	manager.appendLog(format, values...)
}

func (manager *Manager) paths() paths {
	runtimeRoot := filepath.Join(manager.root, "runtime")
	return paths{
		config:       filepath.Join(manager.root, "configs", "server-directory-local-ui.json"),
		network:      filepath.Join(manager.root, "configs", "network.json"),
		ports:        filepath.Join(manager.root, "configs", "ports.json"),
		gmAuth:       filepath.Join(manager.root, "configs", "gm-auth.json"),
		state:        filepath.Join(runtimeRoot, "run-state.json"),
		server:       filepath.Join(runtimeRoot, "bin", "qqt-server-local.exe"),
		clientRoot:   filepath.Join(runtimeRoot, "client-patched"),
		client:       filepath.Join(runtimeRoot, "client-patched", "Client.exe"),
		launcher:     filepath.Join(runtimeRoot, "bin", "qqt-launch-local.exe"),
		loginHelper:  filepath.Join(runtimeRoot, "bin", "qqt-login-immediate.exe"),
		shopHelper:   filepath.Join(runtimeRoot, "bin", "qqt-shop-server-probe.exe"),
		trace:        filepath.Join(manager.root, "configs", "sso-message-trace-stable.json"),
		logs:         filepath.Join(runtimeRoot, "logs"),
		serverStdout: filepath.Join(runtimeRoot, "logs", "local-server.stdout.log"),
		serverStderr: filepath.Join(runtimeRoot, "logs", "local-server.stderr.log"),
		launcherLog:  filepath.Join(runtimeRoot, "logs", "launcher-ui.log"),
	}
}

type paths struct {
	config, network, gmAuth, state, server, clientRoot, client, launcher, ports string
	loginHelper, shopHelper, trace, logs, serverStdout, serverStderr, launcherLog string
}

func (manager *Manager) LoadSettings() (networksetup.Settings, error) {
	return networksetup.Load(manager.paths().network)
}

func (manager *Manager) SaveGMCredentials(username, password string) error {
	credentials, err := gm.NewCredentials(username, password)
	if err != nil {
		return err
	}
	return gm.SaveCredentials(manager.paths().gmAuth, credentials)
}

func (manager *Manager) Status() Status {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	settings, err := networksetup.Load(manager.paths().network)
	if err != nil {
		settings = networksetup.Default()
	}
	state, _ := manager.loadState()
	status := Status{Settings: settings, GMURL: gmURL(settings, manager.paths().ports)}
	if settings.GMRemote {
		status.GMURL = gmURL(settings, manager.paths().ports)
	}
	if state.Server != nil && processMatches(state.Server.PID, manager.paths().server) {
		status.ServerRunning = true
		status.ServerPID = state.Server.PID
	} else if matches := processesByPath(manager.paths().server); len(matches) != 0 {
		status.ServerRunning = true
		status.ServerPID = matches[0]
	}
	state = manager.reconcileClients(state)
	status.ClientCount = len(state.Clients)
	return status
}

func (manager *Manager) StartServer(settings networksetup.Settings) (Status, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	paths := manager.paths()
	if len(processesByPath(paths.server)) != 0 {
		current, err := networksetup.Load(paths.network)
		if err != nil {
			current = settings
		}
		return manager.statusLocked(current), nil
	}
	if err := networksetup.Save(paths.network, settings); err != nil {
		return Status{}, fmt.Errorf("淇濆瓨鑱旀満璁剧疆：%w", err)
	}
	if err := networksetup.ApplyClientTarget(paths.clientRoot, settings.ClientServerIP); err != nil {
		return Status{}, fmt.Errorf("鏇存柊瀹㈡埛绔湇鍔″櫒鍦板潃：%w", err)
	}
	if settings.GMRemote {
		if _, err := gm.LoadCredentials(paths.gmAuth); err != nil {
			return Status{}, errors.New("寮€鏀捐繙绋?GM 鍓嶏紝璇峰厛鍦ㄤ笅鏂逛繚瀛橀潪绌虹殑 GM 瀵嗙爜")
		}
	}
	for _, required := range []string{paths.server, paths.config, paths.network, filepath.Join(paths.clientRoot, "config", "Commodity.ini")} {
		if info, err := os.Stat(required); err != nil || info.IsDir() {
			return Status{}, fmt.Errorf("鍚姩渚濊禆缂哄け：%s", required)
		}
	}
	if err := os.MkdirAll(paths.logs, 0o755); err != nil {
		return Status{}, fmt.Errorf("鍒涘缓鏃ュ織鐩綍：%w", err)
	}
	stdout, err := os.OpenFile(paths.serverStdout, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return Status{}, fmt.Errorf("鎵撳紑鏈嶅姟绔緭鍑烘棩蹇楋細%w", err)
	}
	stderr, err := os.OpenFile(paths.serverStderr, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		stdout.Close()
		return Status{}, fmt.Errorf("鎵撳紑鏈嶅姟绔敊璇棩蹇楋細%w", err)
	}
	command := exec.Command(paths.server, "-config", paths.config, "-ports", paths.ports)
	command.Dir = manager.root
	command.Stdout = stdout
	command.Stderr = stderr
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := command.Start(); err != nil {
		stdout.Close()
		stderr.Close()
		return Status{}, fmt.Errorf("鍚姩 QQ鍫傛湇鍔＄：%w", err)
	}
	stdout.Close()
	stderr.Close()
	pid := command.Process.Pid
	readyErr := waitForServerReady(command, paths.serverStdout, paths.serverStderr, serverReadyTimeout)
	_ = command.Process.Release()
	if readyErr != nil {
		_ = terminateExactProcess(pid, paths.server)
		return Status{}, readyErr
	}
	state, _ := manager.loadState()
	if state.StartedUTC == "" {
		state.StartedUTC = time.Now().UTC().Format(time.RFC3339Nano)
	}
	state.SchemaVersion = stateSchemaVersion
	state.Config = paths.config
	state.Network = settings
	state.Server = &ProcessEntry{PID: pid, Path: paths.server, StartTimeUTC: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := manager.saveState(state); err != nil {
		_ = terminateExactProcess(pid, paths.server)
		return Status{}, fmt.Errorf("鏈嶅姟宸插惎鍔紝浣嗕繚瀛樿繍琛岀姸鎬佸け璐ワ細%w", err)
	}
	manager.appendLog("鏈嶅姟绔凡鍚姩 PID=%d mode=%s ip=%s", pid, settings.Mode, settings.ServerIP)
	return manager.statusLocked(settings), nil
}

func (manager *Manager) StopServer() (Status, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	paths := manager.paths()
	state, _ := manager.loadState()
	pids := processesByPath(paths.server)
	var failures []string
	graceful := 0
	for _, pid := range pids {
		stoppedGracefully, err := stopExactServerProcess(pid, paths.server, 10*time.Second)
		if err != nil {
			failures = append(failures, fmt.Sprintf("PID %d: %v", pid, err))
		} else if stoppedGracefully {
			graceful++
		}
	}
	if len(failures) != 0 {
		return Status{}, fmt.Errorf("鍋滄鏈嶅姟绔け璐ワ細%s", strings.Join(failures, "; "))
	}
	state.Server = nil
	state.SchemaVersion = stateSchemaVersion
	if err := manager.saveState(state); err != nil {
		return Status{}, fmt.Errorf("鏈嶅姟绔凡鍋滄锛屼絾淇濆瓨杩愯鐘舵€佸け璐ワ細%w", err)
	}
	settings, _ := networksetup.Load(paths.network)
	manager.appendLog("鏈嶅姟绔仠姝㈠畬鎴愶紙鎵惧埌 %d 涓繘绋嬶紝浼橀泤鍏抽棴 %d 涓級", len(pids), graceful)
	return manager.statusLocked(settings), nil
}

func waitForServerReady(command *exec.Cmd, stdoutPath, stderrPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if command.ProcessState != nil && command.ProcessState.Exited() {
			return fmt.Errorf("鏈嶅姟绔惎鍔ㄥ悗绔嬪嵆閫€鍑猴細%s", tailFile(stderrPath, 20))
		}
		if data, err := os.ReadFile(stdoutPath); err == nil && strings.Contains(string(data), `"event":"server_ready"`) {
			return nil
		}
		if !processAlive(command.Process.Pid) {
			return fmt.Errorf("鏈嶅姟绔惎鍔ㄥ悗绔嬪嵆閫€鍑猴細%s", tailFile(stderrPath, 20))
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("服务端在 %s 内未就绪。请查看 %s：%s", timeout.Round(time.Second), stderrPath, tailFile(stderrPath, 20))
}

func gmURL(settings networksetup.Settings, portsPath string) string {
	ports, err := networksetup.LoadPorts(portsPath)
	if err != nil {
		ports = networksetup.DefaultPorts()
	}
	host := "127.0.0.1"
	if settings.GMRemote {
		host = settings.ServerIP
	}
	return fmt.Sprintf("http://%s:%d/gm/", host, ports.GM)
}

func (manager *Manager) statusLocked(settings networksetup.Settings) Status {
	state, _ := manager.loadState()
	status := Status{Settings: settings, GMURL: gmURL(settings, manager.paths().ports)}
	if settings.GMRemote {
		status.GMURL = gmURL(settings, manager.paths().ports)
	}
	if matches := processesByPath(manager.paths().server); len(matches) != 0 {
		status.ServerRunning, status.ServerPID = true, matches[0]
	}
	state = manager.reconcileClients(state)
	status.ClientCount = len(state.Clients)
	return status
}

func (manager *Manager) loadState() (RunState, error) {
	data, err := os.ReadFile(manager.paths().state)
	if errors.Is(err, os.ErrNotExist) {
		settings, _ := networksetup.Load(manager.paths().network)
		return RunState{SchemaVersion: stateSchemaVersion, Network: settings, Clients: []ProcessEntry{}, LoginHelpers: []ProcessEntry{}, ShopServerHelpers: []ProcessEntry{}}, nil
	}
	if err != nil {
		return RunState{}, err
	}
	var state RunState
	if err := json.Unmarshal(data, &state); err != nil {
		return RunState{}, err
	}
	return state, nil
}

func (manager *Manager) saveState(state RunState) error {
	state.SchemaVersion = stateSchemaVersion
	if state.Clients == nil {
		state.Clients = []ProcessEntry{}
	}
	if state.LoginHelpers == nil {
		state.LoginHelpers = []ProcessEntry{}
	}
	if state.ShopServerHelpers == nil {
		state.ShopServerHelpers = []ProcessEntry{}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(manager.paths().state, append(data, '\n'))
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".launcher-state-*.json")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err = temporary.Write(data); err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(path)
		return os.Rename(name, path)
	}
	return nil
}

func (manager *Manager) reconcileClients(state RunState) RunState {
	live := make(map[int]struct{})
	filtered := state.Clients[:0]
	for _, entry := range state.Clients {
		if processMatches(entry.PID, manager.paths().client) {
			filtered = append(filtered, entry)
			live[entry.Instance] = struct{}{}
		}
	}
	state.Clients = filtered
	filterHelpers := func(entries []ProcessEntry, expected string) []ProcessEntry {
		result := entries[:0]
		for _, entry := range entries {
			if _, ok := live[entry.Instance]; ok && processMatches(entry.PID, expected) {
				result = append(result, entry)
			}
		}
		return result
	}
	state.LoginHelpers = filterHelpers(state.LoginHelpers, manager.paths().loginHelper)
	state.ShopServerHelpers = filterHelpers(state.ShopServerHelpers, manager.paths().shopHelper)
	state.ClientCount = len(state.Clients)
	if len(state.Clients) != 0 {
		state.Client = &state.Clients[0]
	} else {
		state.Client = nil
	}
	if len(state.LoginHelpers) != 0 {
		state.LoginHelper = &state.LoginHelpers[0]
	} else {
		state.LoginHelper = nil
	}
	return state
}

func (manager *Manager) appendLog(format string, values ...any) {
	_ = os.MkdirAll(manager.paths().logs, 0o755)
	file, err := os.OpenFile(manager.paths().launcherLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = fmt.Fprintf(file, "%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, values...))
}

func tailFile(path string, lines int) string {
	file, err := os.Open(path)
	if err != nil {
		return err.Error()
	}
	defer file.Close()
	var values []string
	scanner := bufio.NewScanner(io.LimitReader(file, 4<<20))
	for scanner.Scan() {
		values = append(values, scanner.Text())
		if len(values) > lines {
			values = values[1:]
		}
	}
	return strings.Join(values, " | ")
}

func waitForFile(ctx context.Context, path string, valid func([]byte) (bool, error)) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if data, err := os.ReadFile(path); err == nil {
			ok, validateErr := valid(data)
			if validateErr != nil {
				return validateErr
			}
			if ok {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func serverReachable(ip string, directoryPort int) error {
	connection, err := net.DialTimeout("tcp", net.JoinHostPort(ip, fmt.Sprint(directoryPort)), 5*time.Second)
	if err != nil {
		return err
	}
	return connection.Close()
}

func instanceFile(logs, name string, instance int) string {
	if instance == 1 {
		return filepath.Join(logs, name)
	}
	extension := filepath.Ext(name)
	base := strings.TrimSuffix(name, extension)
	return filepath.Join(logs, base+"-"+strconv.Itoa(instance)+extension)
}



