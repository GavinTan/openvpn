package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gavintan/gopkg/aes"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
)

type SysBeseConfig struct {
	SiteUrl              string `json:"site_url" mapstructure:"site_url"`
	WebPort              string `json:"web_port" mapstructure:"web_port"`
	AdminUsername        string `json:"admin_username" mapstructure:"admin_username"`
	AdminPassword        string `json:"admin_password" mapstructure:"admin_password"`
	AutoUpdateOvpnConfig bool   `json:"auto_update_ovpn_config" mapstructure:"auto_update_ovpn_config"`
	MaxDuplicateLogin    int    `json:"max_duplicate_login" mapstructure:"max_duplicate_login"`
	HistoryMaxDays       int    `json:"history_max_days" mapstructure:"history_max_days"`
	ValidateClientConfig bool   `json:"validate_client_config" mapstructure:"validate_client_config"`
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

type SysEmailConfig struct {
	SendSubjectPrefix  string  `json:"send_subject_prefix" mapstructure:"send_subject_prefix"`
	SendFrom           string  `json:"send_from" mapstructure:"send_from"`
	Host               string  `json:"host" mapstructure:"host"`
	Port               int     `json:"port" mapstructure:"port"`
	Username           string  `json:"username" mapstructure:"username"`
	Password           string  `json:"password" mapstructure:"password"`
	Security           *string `json:"security" mapstructure:"security"`
	LoginLinkEnabled   bool    `json:"login_link_enabled" mapstructure:"login_link_enabled"` // 邮件是否带登录链接
	HelpURL            string  `json:"help_url" mapstructure:"help_url"`                     // 使用说明外链，附在所有邮件模板
}

// SysFeishuConfig 是飞书组织架构同步的配置。AppSecret 在落盘时 AES 加密，
// loadConfig 时解密到包级变量 feishuAppSecret。
type SysFeishuConfig struct {
	Enabled        bool   `json:"feishu_enabled" mapstructure:"feishu_enabled"`
	AppID          string `json:"feishu_app_id" mapstructure:"feishu_app_id"`
	AppSecret      string `json:"feishu_app_secret" mapstructure:"feishu_app_secret"` // AES 密文
	BaseURL        string `json:"feishu_base_url" mapstructure:"feishu_base_url"`     // 空则用默认 https://open.feishu.cn
	RootDeptID     string `json:"feishu_root_dept_id" mapstructure:"feishu_root_dept_id"`
	SyncCron       string `json:"feishu_sync_cron" mapstructure:"feishu_sync_cron"`
	DisableOnLeave bool   `json:"feishu_disable_on_leave" mapstructure:"feishu_disable_on_leave"`
	DefaultGroupID uint   `json:"feishu_default_group_id" mapstructure:"feishu_default_group_id"`
	NotifyOnCreate bool   `json:"feishu_notify_on_create" mapstructure:"feishu_notify_on_create"`
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
	OvpnPushDns1   string `json:"ovpn_push_dns1" mapstructure:"ovpn_push_dns1"`
	OvpnPushDns2   string `json:"ovpn_push_dns2" mapstructure:"ovpn_push_dns2"`
	OvpnRemoteAddr string `json:"ovpn_remote_addr" mapstructure:"ovpn_remote_addr"` // 外显外网 IP（NAT 场景），生成 ovpn 用
	OvpnRemotePort string `json:"ovpn_remote_port" mapstructure:"ovpn_remote_port"` // 外显外网端口
}

type config struct {
	System struct {
		Base   SysBeseConfig  `json:"base" mapstructure:"base"`
		Ldap   SysLdapConfig  `json:"ldap" mapstructure:"ldap"`
		Email  SysEmailConfig `json:"email" mapstructure:"email"`
		Feishu SysFeishuConfig `json:"feishu" mapstructure:"feishu"`
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
	nftTableName           string

	// 飞书同步运行时配置（app_secret 在 loadConfig 中解密）
	feishuEnabled        bool
	feishuAppID          string
	feishuAppSecret      string
	feishuBaseURL        string
	feishuRootDeptID     string
	feishuSyncCron       string
	feishuDisableOnLeave bool
	feishuDefaultGroupID uint
	feishuNotifyOnCreate bool

	ovManage string
)

func initConfig() {
	sk := genRandomString(50)
	passwd, _ := bcrypt.GenerateFromPassword([]byte("admin"), 12)

	viper.SetDefault("system.base.site_url", envOr("OVPN_SITE_URL", "http://127.0.0.1:8833"))
	viper.SetDefault("system.base.web_port", "8833")
	viper.SetDefault("system.base.secret_key", sk)
	viper.SetDefault("system.base.server_cn", "ovpn_"+genRandomString(16))
	viper.SetDefault("system.base.server_name", "server_"+genRandomString(16))
	viper.SetDefault("system.base.admin_username", "admin")
	viper.SetDefault("system.base.admin_password", string(passwd))
	viper.SetDefault("system.base.auto_update_ovpn_config", false)
	viper.SetDefault("system.base.max_duplicate_login", 0)
	viper.SetDefault("system.base.validate_client_config", false)
	viper.SetDefault("system.base.history_max_days", 90)
	viper.SetDefault("system.base.nft_table_name", "openvpn-nft")
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
	viper.SetDefault("system.email.send_subject_prefix", "【openvpn-web】")
	viper.SetDefault("system.email.send_from", "")
	viper.SetDefault("system.email.host", "")
	viper.SetDefault("system.email.port", 25)
	viper.SetDefault("system.email.username", "")
	viper.SetDefault("system.email.password", "")
	viper.SetDefault("system.email.security", nil)
	viper.SetDefault("system.email.login_link_enabled", envOrBool("OVPN_EMAIL_LOGIN_LINK", true))
	viper.SetDefault("system.email.help_url", envOr("OVPN_HELP_URL", ""))

	// 飞书组织架构同步
	viper.SetDefault("system.feishu.feishu_enabled", false)
	viper.SetDefault("system.feishu.feishu_app_id", "")
	viper.SetDefault("system.feishu.feishu_app_secret", "")
	viper.SetDefault("system.feishu.feishu_base_url", "")
	viper.SetDefault("system.feishu.feishu_root_dept_id", "0")
	viper.SetDefault("system.feishu.feishu_sync_cron", "0 2 * * *")
	viper.SetDefault("system.feishu.feishu_disable_on_leave", true)
	viper.SetDefault("system.feishu.feishu_default_group_id", 1)
	viper.SetDefault("system.feishu.feishu_notify_on_create", true)

	viper.SetDefault("client.client_url.windows", "https://openvpn.net/downloads/openvpn-connect-v3-windows.msi")
	viper.SetDefault("client.client_url.macos", "https://openvpn.net/downloads/openvpn-connect-v3-macos.dmg")
	viper.SetDefault("client.client_url.linux", "https://openvpn.net/openvpn-client-for-linux/")
	viper.SetDefault("client.client_url.android", "https://play.google.com/store/apps/details?id=net.openvpn.openvpn")
	viper.SetDefault("client.client_url.ios", "https://itunes.apple.com/us/app/openvpn-connect/id590379981?mt=8")

	viper.SetDefault("openvpn.ovpn_port", 1194)
	viper.SetDefault("openvpn.ovpn_proto", "udp")
	viper.SetDefault("openvpn.ovpn_subnet", "10.8.0.0/24")
	viper.SetDefault("openvpn.ovpn_max_clients", 200)
	viper.SetDefault("openvpn.ovpn_gateway", false)
	viper.SetDefault("openvpn.ovpn_management", "127.0.0.1:7505")
	viper.SetDefault("openvpn.ovpn_ipv6", false)
	viper.SetDefault("openvpn.ovpn_subnet6", "fdaf:f178:e916:6dd0::/64")
	viper.SetDefault("openvpn.ovpn_push_dns1", "8.8.8.8")
	viper.SetDefault("openvpn.ovpn_push_dns2", "2001:4860:4860::8888")
	viper.SetDefault("openvpn.ovpn_remote_addr", envOr("OVPN_REMOTE_ADDR", ""))
	viper.SetDefault("openvpn.ovpn_remote_port", envOr("OVPN_REMOTE_PORT", ""))

	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.SetConfigPermissions(0600)
	viper.AddConfigPath(ovData)

	viper.SafeWriteConfig()

	err := viper.ReadInConfig()
	if err != nil {
		panic(err)
	}

	if !viper.IsSet("system.base.token") {
		viper.Set("system.base.token", "ovpntoken"+genRandomString(16))
		viper.WriteConfig()
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
		restartFeishuCron()
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
			if k == "ovpn_push_dns1" || k == "ovpn_push_dns2" {
				continue
			}

			cfg.Update("openvpn."+k, fmt.Sprintf("%v", v))
		}

		cfg.Set("setenv auth_api", fmt.Sprintf("http://127.0.0.1:%s/login", webPort))
		cfg.Set("setenv ovpn_auth_api", fmt.Sprintf("http://127.0.0.1:%s/ovpn/login", webPort))
		cfg.Set("setenv ovpn_history_api", fmt.Sprintf("http://127.0.0.1:%s/ovpn/history", webPort))
		cfg.Save()
	}
}

func loadConfig() {
	secretKey = viper.GetString("system.base.secret_key")
	nftTableName = viper.GetString("system.base.nft_table_name")

	viper.Unmarshal(&conf)

	webPort = conf.System.Base.WebPort
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

	// 飞书同步配置；app_secret 落盘为 AES 密文，这里解密到明文供运行时使用。
	// 解密失败（如默认空串）当作空处理，与 email.password 同样的容忍策略。
	feishuEnabled = conf.System.Feishu.Enabled
	feishuAppID = conf.System.Feishu.AppID
	if plain, err := aes.AesDecrypt(conf.System.Feishu.AppSecret, secretKey); err == nil {
		feishuAppSecret = plain
	} else {
		feishuAppSecret = ""
	}
	feishuBaseURL = conf.System.Feishu.BaseURL
	feishuRootDeptID = conf.System.Feishu.RootDeptID
	feishuSyncCron = conf.System.Feishu.SyncCron
	feishuDisableOnLeave = conf.System.Feishu.DisableOnLeave
	feishuDefaultGroupID = conf.System.Feishu.DefaultGroupID
	feishuNotifyOnCreate = conf.System.Feishu.NotifyOnCreate

	ovManage = conf.Openvpn.OvpnManagement
}

// currentFeishuConfig 把包级运行时变量组装成 FeishuSyncConfig，
// 供 cron 任务与 admin 手动触发复用。
func currentFeishuConfig() FeishuSyncConfig {
	return FeishuSyncConfig{
		AppID:          feishuAppID,
		AppSecret:      feishuAppSecret,
		BaseURL:        feishuBaseURL,
		RootDeptID:     feishuRootDeptID,
		DefaultGroupID: feishuDefaultGroupID,
		DisableOnLeave: feishuDisableOnLeave,
		NotifyOnCreate: feishuNotifyOnCreate,
	}
}

// envOr 取环境变量，空则返回默认值。供 initConfig 把部署期环境变量注入为 viper 默认值。
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envOrBool 取布尔环境变量（"true"/"1" 为真），未设则返回默认值。
func envOrBool(key string, def bool) bool {
	switch os.Getenv(key) {
	case "true", "1":
		return true
	case "false", "0":
		return false
	default:
		return def
	}
}
