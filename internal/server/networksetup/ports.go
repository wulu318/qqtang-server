package networksetup

import (
	"encoding/json"
	"fmt"
	"os"
)

// Ports is the single readable source of truth for every listener the server,
// launcher and patched client must agree on. Operators edit configs/ports.json
// once; the launcher rewrites the client INI files from the same values.
type Ports struct {
	Directory int `json:"directory"` // directory HTTP (Dir0/Dir3)
	Game      int `json:"game"`      // game TCP (Dir1/Dir4)
	Shop      int `json:"shop"`      // shop service
	Secondary int `json:"secondary"` // login helper TCP
	TLS       int `json:"tls"`       // directory TLS (Dir2/Dir5)
	GM        int `json:"gm"`        // GM HTTP
	UDP       int `json:"udp"`       // QQTPPP UDP
}

func DefaultPorts() Ports {
	return Ports{Directory: 18080, Game: 18000, Shop: 18001, Secondary: 17000, TLS: 18443, GM: 18100, UDP: 18000}
}

func (ports Ports) withDefaults() Ports {
	defaults := DefaultPorts()
	if ports.Directory == 0 {
		ports.Directory = defaults.Directory
	}
	if ports.Game == 0 {
		ports.Game = defaults.Game
	}
	if ports.Shop == 0 {
		ports.Shop = defaults.Shop
	}
	if ports.Secondary == 0 {
		ports.Secondary = defaults.Secondary
	}
	if ports.TLS == 0 {
		ports.TLS = defaults.TLS
	}
	if ports.GM == 0 {
		ports.GM = defaults.GM
	}
	if ports.UDP == 0 {
		ports.UDP = defaults.UDP
	}
	return ports
}

func (ports Ports) Validate() error {
	for name, value := range map[string]int{
		"directory": ports.Directory, "game": ports.Game, "shop": ports.Shop,
		"secondary": ports.Secondary, "tls": ports.TLS, "gm": ports.GM, "udp": ports.UDP,
	} {
		if value < 1 || value > 65535 {
			return fmt.Errorf("端口 %s=%d 超出范围", name, value)
		}
	}
	return nil
}

// LoadPorts reads configs/ports.json. A missing file means defaults, so an
// existing installation keeps working without the new file.
func LoadPorts(path string) (Ports, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultPorts(), nil
		}
		return Ports{}, err
	}
	var ports Ports
	if err = json.Unmarshal(data, &ports); err != nil {
		return Ports{}, fmt.Errorf("解析端口配置 %s: %w", path, err)
	}
	ports = ports.withDefaults()
	if err = ports.Validate(); err != nil {
		return Ports{}, err
	}
	return ports, nil
}

// SavePorts writes the file in a stable, diff-friendly shape.
func SavePorts(path string, ports Ports) error {
	ports = ports.withDefaults()
	data, err := json.MarshalIndent(ports, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
