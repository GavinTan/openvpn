package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

type VPNConfig struct {
	ConfigPath string
	Lines      []string
	mu         sync.Mutex
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

func (cfg *VPNConfig) Get(key string) (val string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	keyPrefix := key + " "
	for _, line := range cfg.Lines {
		if strings.HasPrefix(line, keyPrefix) {
			return strings.TrimSpace(line[len(keyPrefix):])
		}
	}
	return ""
}

func (cfg *VPNConfig) Set(key, value string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	found := false
	keyPrefix := key + " "
	newLine := fmt.Sprintf("%s %s", key, value)

	for i, line := range cfg.Lines {
		trim := strings.TrimSpace(line)

		isComment := false
		if strings.HasPrefix(trim, "#") {
			isComment = true
			trim = strings.TrimSpace(trim[1:])
		}

		if key == "push" {
			if keyPrefix+value == trim {
				if isComment {
					cfg.Lines[i] = newLine
				}

				found = true
				break
			}
		} else {
			if strings.HasPrefix(trim, keyPrefix) {
				cfg.Lines[i] = newLine
				found = true
				break
			}
		}

	}

	if !found {
		cfg.Lines = append(cfg.Lines, fmt.Sprintf("%s %s", key, value))
	}
}

func (cfg *VPNConfig) SetLine(index int, content string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	if index >= 0 && index < len(cfg.Lines) {
		cfg.Lines[index] = content
	} else {
		cfg.Lines = append(cfg.Lines, content)
	}
}

func (cfg *VPNConfig) Delete(key string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

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

func (cfg *VPNConfig) DeleteLines(indexes []int) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	var newLines []string
	for i, line := range cfg.Lines {
		if !slices.Contains(indexes, i) {
			newLines = append(newLines, line)
		}
	}

	cfg.Lines = newLines
}

func (cfg *VPNConfig) Save() {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	os.WriteFile(cfg.ConfigPath, []byte(strings.Join(cfg.Lines, "\n")+"\n"), 0644)
}

func (cfg *VPNConfig) Update(key string, val string) {
	switch key {
	case "openvpn.ovpn_port":
		cfg.Set("port", val)
	case "openvpn.ovpn_proto":
		cfg.Set("proto", val)
	case "openvpn.ovpn_max_clients":
		cfg.Set("max-clients", val)
	case "openvpn.ovpn_subnet":
		oldSubnet := cfg.Get("server")
		ip, ipnet, err := net.ParseCIDR(val)
		if err != nil {
			logger.Error(context.Background(), err.Error())
			return
		}
		val = fmt.Sprintf("%s %s", ip.String(), net.IP(ipnet.Mask).String())
		cfg.Set("server", val)

		ipt := "iptables-nft"
		checkCmd := exec.Command("iptables-legacy", "-L", "-n", "-t", "nat")
		if err := checkCmd.Run(); err == nil {
			ipt = "iptables-legacy"
		}

		if oldSubnet != "" && oldSubnet != val {
			getOldCmd := exec.Command(ipt, "-t", "nat", "-C", "POSTROUTING", "-s", strings.ReplaceAll(oldSubnet, " ", "/"), "-j", "MASQUERADE")
			if err := getOldCmd.Run(); err == nil {
				delOldCmd := exec.Command(ipt, "-t", "nat", "-D", "POSTROUTING", "-s", strings.ReplaceAll(oldSubnet, " ", "/"), "-j", "MASQUERADE")
				if out, err := delOldCmd.CombinedOutput(); err != nil {
					if len(out) == 0 {
						out = []byte(err.Error())
					}
					logger.Error(context.Background(), string(out))
				}
			}
		}

		getCmd := exec.Command(ipt, "-t", "nat", "-C", "POSTROUTING", "-s", strings.ReplaceAll(val, " ", "/"), "-j", "MASQUERADE")
		if err := getCmd.Run(); err != nil {
			addCmd := exec.Command(ipt, "-t", "nat", "-A", "POSTROUTING", "-s", strings.ReplaceAll(val, " ", "/"), "-j", "MASQUERADE")
			if out, err := addCmd.CombinedOutput(); err != nil {
				if len(out) == 0 {
					out = []byte(err.Error())
				}
				logger.Error(context.Background(), string(out))
			}
		}
	case "openvpn.ovpn_gateway":
		if val == "true" {
			var dnsIndices []int
			for i, line := range cfg.Lines {
				if strings.Contains(line, "dhcp-option DNS") {
					dnsIndices = append(dnsIndices, i)
				}
			}

			if len(dnsIndices) > 0 {
				if len(dnsIndices) == 1 {
					cfg.SetLine(dnsIndices[0], fmt.Sprintf(`push "dhcp-option DNS %s"`, viper.GetString("openvpn.ovpn_push_dns1")))
					cfg.SetLine(len(cfg.Lines), fmt.Sprintf(`push "dhcp-option DNS %s"`, viper.GetString("openvpn.ovpn_push_dns2")))
				} else if len(dnsIndices) == 2 {
					cfg.SetLine(dnsIndices[0], fmt.Sprintf(`push "dhcp-option DNS %s"`, viper.GetString("openvpn.ovpn_push_dns1")))
					cfg.SetLine(dnsIndices[1], fmt.Sprintf(`push "dhcp-option DNS %s"`, viper.GetString("openvpn.ovpn_push_dns2")))
				} else {
					cfg.SetLine(dnsIndices[0], fmt.Sprintf(`push "dhcp-option DNS %s"`, viper.GetString("openvpn.ovpn_push_dns1")))
					cfg.SetLine(dnsIndices[1], fmt.Sprintf(`push "dhcp-option DNS %s"`, viper.GetString("openvpn.ovpn_push_dns2")))
					cfg.DeleteLines(dnsIndices[2:])
				}
			} else {
				cfg.SetLine(len(cfg.Lines), fmt.Sprintf(`push "dhcp-option DNS %s"`, viper.GetString("openvpn.ovpn_push_dns1")))
				cfg.SetLine(len(cfg.Lines), fmt.Sprintf(`push "dhcp-option DNS %s"`, viper.GetString("openvpn.ovpn_push_dns2")))
			}

			cfg.Set("push", `"redirect-gateway def1 ipv6 bypass-dhcp"`)
		} else {
			cfg.Delete(fmt.Sprintf(`push "dhcp-option DNS %s"`, viper.GetString("openvpn.ovpn_push_dns1")))
			cfg.Delete(fmt.Sprintf(`push "dhcp-option DNS %s"`, viper.GetString("openvpn.ovpn_push_dns2")))
			cfg.Delete(`push "redirect-gateway def1 ipv6 bypass-dhcp"`)
		}
	case "openvpn.ovpn_management":
		cfg.Set("management", strings.ReplaceAll(val, ":", " "))
	case "openvpn.ovpn_ipv6":
		ipt := "ip6tables-nft"
		checkCmd := exec.Command("ip6tables-legacy", "-L", "-n", "-t", "nat")
		if err := checkCmd.Run(); err == nil {
			ipt = "ip6tables-legacy"
		}

		if val == "true" {
			proto := conf.Openvpn.OvpnProto
			if !strings.HasSuffix(proto, "6") {
				proto = fmt.Sprintf("%s6", proto)
			}
			cfg.Set("proto", proto)
			cfg.Set("server-ipv6", conf.Openvpn.OvpnSubnet6)

			getCmd := exec.Command(ipt, "-t", "nat", "-C", "POSTROUTING", "-s", conf.Openvpn.OvpnSubnet6, "-j", "MASQUERADE")
			if err := getCmd.Run(); err != nil {
				addCmd := exec.Command(ipt, "-t", "nat", "-A", "POSTROUTING", "-s", conf.Openvpn.OvpnSubnet6, "-j", "MASQUERADE")
				if out, err := addCmd.CombinedOutput(); err != nil {
					if len(out) == 0 {
						out = []byte(err.Error())
					}
					logger.Error(context.Background(), string(out))
				}
			}
		} else {
			cfg.Set("proto", conf.Openvpn.OvpnProto)
			cfg.Delete("server-ipv6")

			getCmd := exec.Command(ipt, "-t", "nat", "-C", "POSTROUTING", "-s", conf.Openvpn.OvpnSubnet6, "-j", "MASQUERADE")
			if err := getCmd.Run(); err == nil {
				delCmd := exec.Command(ipt, "-t", "nat", "-D", "POSTROUTING", "-s", conf.Openvpn.OvpnSubnet6, "-j", "MASQUERADE")
				if out, err := delCmd.CombinedOutput(); err != nil {
					if len(out) == 0 {
						out = []byte(err.Error())
					}
					logger.Error(context.Background(), string(out))
				}
			}
		}
	case "openvpn.ovpn_subnet6":
		if viper.GetBool("openvpn.ovpn_ipv6") {
			oldSubnet6 := cfg.Get("server-ipv6")

			cfg.Set("server-ipv6", val)

			ipt := "ip6tables-nft"
			checkCmd := exec.Command("ip6tables-legacy", "-L", "-n", "-t", "nat")
			if err := checkCmd.Run(); err == nil {
				ipt = "ip6tables-legacy"
			}

			if oldSubnet6 != "" && oldSubnet6 != val {
				getOldCmd := exec.Command(ipt, "-t", "nat", "-C", "POSTROUTING", "-s", oldSubnet6, "-j", "MASQUERADE")
				if err := getOldCmd.Run(); err == nil {
					delOldCmd := exec.Command(ipt, "-t", "nat", "-D", "POSTROUTING", "-s", oldSubnet6, "-j", "MASQUERADE")
					if out, err := delOldCmd.CombinedOutput(); err != nil {
						if len(out) == 0 {
							out = []byte(err.Error())
						}
						logger.Error(context.Background(), string(out))
					}
				}
			}

			getCmd := exec.Command(ipt, "-t", "nat", "-C", "POSTROUTING", "-s", val, "-j", "MASQUERADE")
			if err := getCmd.Run(); err != nil {
				addCmd := exec.Command(ipt, "-t", "nat", "-A", "POSTROUTING", "-s", val, "-j", "MASQUERADE")
				if out, err := addCmd.CombinedOutput(); err != nil {
					if len(out) == 0 {
						out = []byte(err.Error())
					}
					logger.Error(context.Background(), string(out))
				}
			}
		}
	case "openvpn.ovpn_push_dns1":
		var dnsIndices []int
		for i, line := range cfg.Lines {
			if strings.Contains(line, "dhcp-option DNS") {
				dnsIndices = append(dnsIndices, i)
			}
		}

		if len(dnsIndices) > 0 {
			cfg.SetLine(dnsIndices[0], fmt.Sprintf(`push "dhcp-option DNS %s"`, val))
		} else {
			cfg.SetLine(len(cfg.Lines), fmt.Sprintf(`push "dhcp-option DNS %s"`, val))
		}
	case "openvpn.ovpn_push_dns2":
		var dnsIndices []int
		for i, line := range cfg.Lines {
			if strings.Contains(line, "dhcp-option DNS") {
				dnsIndices = append(dnsIndices, i)
			}
		}

		if len(dnsIndices) > 1 {
			cfg.SetLine(dnsIndices[1], fmt.Sprintf(`push "dhcp-option DNS %s"`, val))
		} else {
			cfg.SetLine(len(cfg.Lines), fmt.Sprintf(`push "dhcp-option DNS %s"`, val))
		}
	}

	cfg.Save()
}
