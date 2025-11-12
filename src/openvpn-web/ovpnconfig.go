package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"path"
	"strings"

	"github.com/spf13/viper"
)

type VPNConfig struct {
	ConfigPath string
	Lines      []string
}

func initOvpnConfig() (*VPNConfig, error) {
	configPath := path.Join(ovData, "server.conf")

	f, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return &VPNConfig{ConfigPath: configPath, Lines: lines}, scanner.Err()
}

func (cfg *VPNConfig) Set(key, value string) {
	keyPrefix := key + " "
	found := false

	for i, line := range cfg.Lines {
		trim := strings.TrimSpace(line)

		if key == "push" {
			if keyPrefix+value == trim {
				found = true
				break
			}
		} else {
			if strings.HasPrefix(trim, keyPrefix) {
				cfg.Lines[i] = fmt.Sprintf("%s %s", key, value)
				found = true
				break
			}
		}

	}

	if !found {
		cfg.Lines = append(cfg.Lines, fmt.Sprintf("%s %s", key, value))
	}
}

func (cfg *VPNConfig) Delete(key string) {
	keyPrefix := key + " "
	var newLines []string

	for _, line := range cfg.Lines {
		trim := strings.TrimSpace(line)

		if strings.HasPrefix(trim, "push") {
			if key == trim {
				continue
			}
		} else {
			if strings.HasPrefix(trim, keyPrefix) {
				continue
			}
		}

		newLines = append(newLines, line)
	}

	cfg.Lines = newLines
}

func (cfg *VPNConfig) Save() {
	os.WriteFile(cfg.ConfigPath, []byte(strings.Join(cfg.Lines, "\n")+"\n"), 0644)
}

func (cfg *VPNConfig) Update(key string, val string) {
	switch key {
	case "openvpn.ovpn_port":
		cfg.Set("port", val)
		cfg.Save()
	case "openvpn.ovpn_proto":
		cfg.Set("proto", val)
		cfg.Save()
	case "openvpn.ovpn_max_clients":
		cfg.Set("max-clients", val)
		cfg.Save()
	case "openvpn.ovpn_subnet":
		ip, ipnet, err := net.ParseCIDR(val)
		if err != nil {
			logger.Error(context.Background(), err.Error())
			return
		}
		val = fmt.Sprintf("%s %s", ip.String(), net.IP(ipnet.Mask).String())
		cfg.Set("server", val)
		cfg.Save()
	case "openvpn.ovpn_gateway":
		if val == "true" {
			cfg.Set("push", `"dhcp-option DNS 8.8.8.8"`)
			cfg.Set("push", `"dhcp-option DNS 2001:4860:4860::8888"`)
			cfg.Set("push", `"redirect-gateway def1 ipv6 bypass-dhcp"`)
			cfg.Save()
		} else {
			cfg.Delete(`push "dhcp-option DNS 8.8.8.8"`)
			cfg.Delete(`push "dhcp-option DNS 2001:4860:4860::8888"`)
			cfg.Delete(`push "redirect-gateway def1 ipv6 bypass-dhcp"`)
			cfg.Save()
		}
	case "openvpn.ovpn_management":
		cfg.Set("management", strings.ReplaceAll(val, ":", " "))
		cfg.Save()
	case "openvpn.ovpn_ipv6":
		if val == "true" {
			cfg.Set("proto", fmt.Sprintf("%s6", getConfig().Openvpn.OvpnProto))
			cfg.Set("server-ipv6", getConfig().Openvpn.OvpnSubnet6)
			cfg.Save()
		} else {
			cfg.Set("proto", getConfig().Openvpn.OvpnProto)
			cfg.Delete("server-ipv6")
			cfg.Save()
		}
	case "openvpn.ovpn_subnet6":
		if viper.GetBool("sopenvpn.ovpn_ipv6") {
			cfg.Set("server-ipv6", val)
			cfg.Save()
		}
	}
}
