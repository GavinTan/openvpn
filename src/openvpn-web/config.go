package main

import (
	"context"
	"fmt"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gavintan/gopkg/aes"
	"github.com/spf13/viper"
)

type SysBeseConfig struct {
	WebPort              string `json:"web_port" mapstructure:"web_port"`
	SecretKey            string `json:"secret_key" mapstructure:"secret_key"`
	ServerCN             string `json:"server_cn" mapstructure:"server_cn"`
	ServerName           string `json:"server_name" mapstructure:"server_name"`
	AdminUsername        string `json:"admin_username" mapstructure:"admin_username"`
	AdminPassword        string `json:"admin_password" mapstructure:"admin_password"`
	AutoUpdateOvpnConfig bool   `json:"auto_update_ovpn_config" mapstructure:"auto_update_ovpn_config"`
}

type SysLdapConfig struct {
	LdapAuth               bool   `json:"ldap_auth" mapstructure:"ldap_auth"`
	LdapUrl                string `json:"ldap_url" mapstructure:"ldap_url"`
	LdapBaseDn             string `json:"ldap_base_dn" mapstructure:"ldap_base_dn"`
	LdapUserAttribute      string `json:"ldap_user_attribute" mapstructure:"ldap_user_attribute"`
	LdapUserGroupFilter    bool   `json:"ldap_user_group_filter" mapstructure:"ldap_user_group_filter"`
	LdapUserGroupDn        string `json:"ldap_user_group_dn" mapstructure:"ldap_user_group_dn"`
	LdapUserAttrIpaddrName string `json:"ldap_user_attr_ipaddr_name" mapstructure:"ldap_user_attr_ipaddr_name"`
	LdapUserAttrConfigName string `json:"ldap_user_attr_config_name" mapstructure:"ldap_user_attr_config_name"`
	LdapBindUserDn         string `json:"ldap_bind_user_dn" mapstructure:"ldap_bind_user_dn"`
	LdapBindPassword       string `json:"ldap_bind_password" mapstructure:"ldap_bind_password"`
}

type ClientUrlConfig struct {
	Windows string `json:"windows" mapstructure:"windows"`
	Linux   string `json:"linux" mapstructure:"linux"`
	Macos   string `json:"macos" mapstructure:"macos"`
	Ios     string `json:"ios" mapstructure:"ios"`
	Android string `json:"android" mapstructure:"android"`
}

type OvpnConfig struct {
	OvpnPort       int    `json:"ovpn_port" mapstructure:"ovpn_port"`
	OvpnProto      string `json:"ovpn_proto" mapstructure:"ovpn_proto"`
	OvpnSubnet     string `json:"ovpn_subnet" mapstructure:"ovpn_subnet"`
	OvpnMaxClients int    `json:"ovpn_max_clients" mapstructure:"ovpn_max_clients"`
	OvpnGateway    bool   `json:"ovpn_gateway" mapstructure:"ovpn_gateway"`
	OvpnManagement string `json:"ovpn_management" mapstructure:"ovpn_management"`
	OvpnIpv6       bool   `json:"ovpn_ipv6" mapstructure:"ovpn_ipv6"`
	OvpnSubnet6    string `json:"ovpn_subnet6" mapstructure:"ovpn_subnet6"`
}

type config struct {
	System struct {
		Base SysBeseConfig `json:"base" mapstructure:"base"`
		Ldap SysLdapConfig `json:"ldap" mapstructure:"ldap"`
	} `json:"system" mapstructure:"system"`
	Client struct {
		ClientUrl ClientUrlConfig `json:"client_url" mapstructure:"client_url"`
	} `json:"client" mapstructure:"client"`
	Openvpn OvpnConfig `json:"openvpn" mapstructure:"openvpn"`
}

var (
	webPort                string
	secretKey              string
	adminUsername          string
	adminPassword          string
	ldapAuth               bool
	ldapURL                string
	ldapBaseDn             string
	ldapUserAttribute      string
	ldapUserGroupFilter    bool
	ldapUserGroupDn        string
	ldapUserAttrIpaddrName string
	ldapUserAttrConfigName string
	ldapBindUserDn         string
	ldapBindPassword       string

	ovManage string
)

func initConfig() {
	sk := genRandomString(50)
	dp, _ := aes.AesEncrypt("admin", sk)

	viper.SetDefault("system.base.web_port", "8833")
	viper.SetDefault("system.base.secret_key", sk)
	viper.SetDefault("system.base.server_cn", "ovpn_"+genRandomString(16))
	viper.SetDefault("system.base.server_name", "server_"+genRandomString(16))
	viper.SetDefault("system.base.admin_username", "admin")
	viper.SetDefault("system.base.admin_password", dp)
	viper.SetDefault("system.base.auto_update_ovpn_config", false)
	viper.SetDefault("system.ldap.ldap_auth", false)
	viper.SetDefault("system.ldap.ldap_url", "ldap://example.org:389")
	viper.SetDefault("system.ldap.ldap_base_dn", "dc=example,dc=org")
	viper.SetDefault("system.ldap.ldap_user_attribute", "uid")
	viper.SetDefault("system.ldap.ldap_user_group_filter", false)
	viper.SetDefault("system.ldap.ldap_user_group_dn", "cn=vpn,ou=groups,dc=example,dc=org")
	viper.SetDefault("system.ldap.ldap_user_attr_ipaddr_name", "ipaddr")
	viper.SetDefault("system.ldap.ldap_user_attr_config_name", "config")
	viper.SetDefault("system.ldap.ldap_bind_user_dn", "cn=admin,dc=example,dc=org")
	viper.SetDefault("system.ldap.ldap_bind_password", "adminpassword")

	viper.SetDefault("client.client_url.windows", "https://openvpn.net/downloads/openvpn-connect-v3-windows.msi")
	viper.SetDefault("client.client_url.macos", "https://openvpn.net/downloads/openvpn-connect-v3-macos.dmg")
	viper.SetDefault("client.client_url.linux", "https://openvpn.net/openvpn-client-for-linux/")
	viper.SetDefault("client.client_url.ios", "https://play.google.com/store/apps/details?id=net.openvpn.openvpn")
	viper.SetDefault("client.client_url.android", "https://itunes.apple.com/us/app/openvpn-connect/id590379981?mt=8")

	viper.SetDefault("openvpn.ovpn_port", 1194)
	viper.SetDefault("openvpn.ovpn_proto", "udp")
	viper.SetDefault("openvpn.ovpn_subnet", "10.8.0.0/24")
	viper.SetDefault("openvpn.ovpn_max_clients", 200)
	viper.SetDefault("openvpn.ovpn_gateway", false)
	viper.SetDefault("openvpn.ovpn_management", "127.0.0.1:7505")
	viper.SetDefault("openvpn.ovpn_ipv6", false)
	viper.SetDefault("openvpn.ovpn_subnet6", "fdaf:f178:e916:6dd0::/64")

	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.AddConfigPath(".")

	err := viper.ReadInConfig()
	if err != nil {
		viper.SafeWriteConfig()
	}

	var lastEventTime time.Time
	viper.OnConfigChange(func(e fsnotify.Event) {
		now := time.Now()
		if now.Sub(lastEventTime) < 500*time.Millisecond {
			return
		}
		lastEventTime = now

		loadConfig()
		upadteOvpnConfig()
	})

	viper.WatchConfig()
}

func upadteOvpnConfig() {
	if viper.GetBool("system.base.auto_update_ovpn_config") {
		cfg, err := initOvpnConfig()
		if err != nil {
			logger.Error(context.Background(), err.Error())
			return
		}

		for k, v := range viper.GetStringMap("openvpn") {
			cfg.Update("openvpn."+k, fmt.Sprintf("%v", v))
		}

		cfg.Set("setenv auth_api", fmt.Sprintf("http://127.0.0.1:%s/login", webPort))
		cfg.Set("setenv ovpn_auth_api", fmt.Sprintf("http://127.0.0.1:%s/ovpn/login", webPort))
		cfg.Set("setenv ovpn_history_api", fmt.Sprintf("http://127.0.0.1:%s/ovpn/history", webPort))
		cfg.Save()
	}
}

func loadConfig() {
	var conf config
	viper.Unmarshal(&conf)

	webPort = conf.System.Base.WebPort
	secretKey = conf.System.Base.SecretKey
	adminUsername = conf.System.Base.AdminUsername
	adminPassword = conf.System.Base.AdminPassword
	ldapAuth = conf.System.Ldap.LdapAuth
	ldapURL = conf.System.Ldap.LdapUrl
	ldapBaseDn = conf.System.Ldap.LdapBaseDn
	ldapUserAttribute = conf.System.Ldap.LdapUserAttribute
	ldapUserGroupFilter = conf.System.Ldap.LdapUserGroupFilter
	ldapUserGroupDn = conf.System.Ldap.LdapUserGroupDn
	ldapUserAttrIpaddrName = conf.System.Ldap.LdapUserAttrIpaddrName
	ldapUserAttrConfigName = conf.System.Ldap.LdapUserAttrConfigName
	ldapBindUserDn = conf.System.Ldap.LdapBindUserDn
	ldapBindPassword = conf.System.Ldap.LdapBindPassword

	ovManage = conf.Openvpn.OvpnManagement
}

func getConfig() (conf config) {
	viper.Unmarshal(&conf)
	return
}
