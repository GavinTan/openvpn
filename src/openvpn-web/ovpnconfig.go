package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
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

func (cfg *VPNConfig) Get(key string) (val string) {
	keyPrefix := key + " "
	for _, line := range cfg.Lines {
		if strings.HasPrefix(line, keyPrefix) {
			return strings.TrimSpace(line[len(keyPrefix):])
		}
	}
	return ""
}

func (cfg *VPNConfig) Set(key, value string) {
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
	if index >= 0 && index < len(cfg.Lines) {
		cfg.Lines[index] = content
	} else {
		cfg.Lines = append(cfg.Lines, content)
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
		oldSubnet := cfg.Get("server")
		ip, ipnet, err := net.ParseCIDR(val)
		if err != nil {
			logger.Error(context.Background(), err.Error())
			return
		}
		val = fmt.Sprintf("%s %s", ip.String(), net.IP(ipnet.Mask).String())
		cfg.Set("server", val)
		cfg.Save()

		ipt := "iptables-nft"
		checkCmd := exec.Command("sh", "-c", "iptables-legacy -L -n -t nat > /dev/null 2>&1")
		if err := checkCmd.Run(); err == nil {
			ipt = "iptables-legacy"
		}

		getCmd := fmt.Sprintf("%s -t nat -C POSTROUTING -s %s -j MASQUERADE > /dev/null 2>&1", ipt, strings.ReplaceAll(oldSubnet, " ", "/"))
		delCmd := fmt.Sprintf("%s -t nat -D POSTROUTING -s %s -j MASQUERADE", ipt, strings.ReplaceAll(oldSubnet, " ", "/"))
		addCmd := fmt.Sprintf("%s -t nat -A POSTROUTING -s %s -j MASQUERADE", ipt, strings.ReplaceAll(val, " ", "/"))
		cmd := exec.Command("sh", "-c", strings.Join([]string{getCmd, delCmd}, " && ")+";"+addCmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			if len(out) == 0 {
				out = []byte(err.Error())
			}
			logger.Error(context.Background(), string(out))
		}
	case "openvpn.ovpn_gateway":
		if val == "true" {
			cfg.Set("push", fmt.Sprintf(`"dhcp-option DNS %s"`, viper.GetString("openvpn.ovpn_push_dns1")))
			cfg.Set("push", fmt.Sprintf(`"dhcp-option DNS %s"`, viper.GetString("openvpn.ovpn_push_dns2")))
			cfg.Set("push", `"redirect-gateway def1 ipv6 bypass-dhcp"`)
			cfg.Save()
		} else {
			cfg.Delete(fmt.Sprintf(`push "dhcp-option DNS %s"`, viper.GetString("openvpn.ovpn_push_dns1")))
			cfg.Delete(fmt.Sprintf(`push "dhcp-option DNS %s"`, viper.GetString("openvpn.ovpn_push_dns2")))
			cfg.Delete(`push "redirect-gateway def1 ipv6 bypass-dhcp"`)
			cfg.Save()
		}
	case "openvpn.ovpn_management":
		cfg.Set("management", strings.ReplaceAll(val, ":", " "))
		cfg.Save()
	case "openvpn.ovpn_ipv6":
		ipt := "ip6tables-nft"
		checkCmd := exec.Command("sh", "-c", "ip6tables-legacy -L -n -t nat > /dev/null 2>&1")
		if err := checkCmd.Run(); err == nil {
			ipt = "ip6tables-legacy"
		}

		if val == "true" {
			cfg.Set("proto", fmt.Sprintf("%s6", getConfig().Openvpn.OvpnProto))
			cfg.Set("server-ipv6", getConfig().Openvpn.OvpnSubnet6)
			cfg.Save()

			getCmd := fmt.Sprintf("%s -t nat -C POSTROUTING -s %s -j MASQUERADE > /dev/null 2>&1", ipt, getConfig().Openvpn.OvpnSubnet6)
			addCmd := fmt.Sprintf("%s -t nat -A POSTROUTING -s %s -j MASQUERADE", ipt, getConfig().Openvpn.OvpnSubnet6)
			cmd := exec.Command("sh", "-c", strings.Join([]string{getCmd, addCmd}, " || "))
			if out, err := cmd.CombinedOutput(); err != nil {
				if len(out) == 0 {
					out = []byte(err.Error())
				}
				logger.Error(context.Background(), string(out))
			}
		} else {
			cfg.Set("proto", getConfig().Openvpn.OvpnProto)
			cfg.Delete("server-ipv6")
			cfg.Save()

			getCmd := fmt.Sprintf("%s -t nat -C POSTROUTING -s %s -j MASQUERADE > /dev/null 2>&1", ipt, getConfig().Openvpn.OvpnSubnet6)
			delCmd := fmt.Sprintf("%s -t nat -D POSTROUTING -s %s -j MASQUERADE", ipt, getConfig().Openvpn.OvpnSubnet6)

			cmd := exec.Command("sh", "-c", strings.Join([]string{getCmd, delCmd}, " && ")+"|| true")
			if out, err := cmd.CombinedOutput(); err != nil {
				if len(out) == 0 {
					out = []byte(err.Error())
				}
				logger.Error(context.Background(), string(out))
			}
		}
	case "openvpn.ovpn_subnet6":
		if viper.GetBool("openvpn.ovpn_ipv6") {
			oldSubnet6 := cfg.Get("server-ipv6")

			cfg.Set("server-ipv6", val)
			cfg.Save()

			ipt := "ip6tables-nft"
			checkCmd := exec.Command("sh", "-c", "ip6tables-legacy -L -n -t nat > /dev/null 2>&1")
			if err := checkCmd.Run(); err == nil {
				ipt = "ip6tables-legacy"
			}

			getOldCmd := fmt.Sprintf("%s -t nat -C POSTROUTING -s %s -j MASQUERADE > /dev/null 2>&1", ipt, oldSubnet6)
			delOldCmd := fmt.Sprintf("%s -t nat -D POSTROUTING -s %s -j MASQUERADE", ipt, oldSubnet6)
			getCmd := fmt.Sprintf("%s -t nat -C POSTROUTING -s %s -j MASQUERADE > /dev/null 2>&1", ipt, val)
			addCmd := fmt.Sprintf("%s -t nat -A POSTROUTING -s %s -j MASQUERADE", ipt, val)

			cmd := exec.Command("sh", "-c", strings.Join([]string{getOldCmd, delOldCmd}, " && ")+";"+strings.Join([]string{getCmd, addCmd}, " || "))
			if out, err := cmd.CombinedOutput(); err != nil {
				if len(out) == 0 {
					out = []byte(err.Error())
				}
				logger.Error(context.Background(), string(out))
			}
		}
	case "openvpn.ovpn_push_dns1":
		var dnsLines []int
		for i, line := range cfg.Lines {
			if strings.Contains(line, "dhcp-option DNS") {
				dnsLines = append(dnsLines, i)
			}
		}

		if len(dnsLines) > 0 {
			cfg.SetLine(dnsLines[0], fmt.Sprintf(`push "dhcp-option DNS %s"`, val))
		} else {
			cfg.SetLine(len(cfg.Lines), fmt.Sprintf(`push "dhcp-option DNS %s"`, val))
		}

		cfg.Save()

	case "openvpn.ovpn_push_dns2":
		var dnsLines []int
		for i, line := range cfg.Lines {
			if strings.Contains(line, "dhcp-option DNS") {
				dnsLines = append(dnsLines, i)
			}
		}

		if len(dnsLines) > 1 {
			cfg.SetLine(dnsLines[1], fmt.Sprintf(`push "dhcp-option DNS %s"`, val))
		} else {
			cfg.SetLine(len(cfg.Lines), fmt.Sprintf(`push "dhcp-option DNS %s"`, val))
		}

		cfg.Save()
	}
}
