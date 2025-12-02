package main

import (
	"bufio"
	"context"
	"crypto/x509"
	"embed"
	"encoding/csv"
	"encoding/pem"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gavintan/gopkg/aes"
	"github.com/gavintan/gopkg/tools"
	"github.com/gin-contrib/sessions"
	gormsessions "github.com/gin-contrib/sessions/gorm"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/patrickmn/go-cache"
	"github.com/spf13/viper"
	"gorm.io/gorm"
	gLogger "gorm.io/gorm/logger"
)

type ClientData struct {
	ID         string `json:"id"`
	Rip        string `json:"rip"`
	Vip        string `json:"vip"`
	Vip6       string `json:"vip6"`
	RecvBytes  string `json:"recvBytes"`
	SendBytes  string `json:"sendBytes"`
	ConnDate   string `json:"connDate"`
	OnlineTime string `json:"onlineTime"`
	UserName   string `json:"username"`
}

type ServerData struct {
	RunDate    string
	Status     string
	StatusDesc string
	Address    string
	Nclients   string
	BytesIn    string
	BytesOut   string
	Mode       string
	Version    string
}

type ClientConfigData struct {
	Name     string `json:"name"`
	FullName string `json:"fullName"`
	File     string `json:"file"`
	Date     string `json:"date"`
}

type Params struct {
	Draw        int    `json:"draw" form:"draw"`
	Offset      int    `json:"offset" form:"offset"`
	Limit       int    `json:"limit" form:"limit"`
	OrderColumn string `json:"orderColumn" form:"orderColumn"`
	Order       string `json:"order" form:"order"`
	Search      string `json:"search" form:"search"`
	Qt          string `json:"qt" form:"qt"`
}

type CertData struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Subject   string `json:"subject"`
	Issuer    string `json:"issuer"`
	NotBefore string `json:"notBefore"`
	NotAfter  string `json:"notAfter"`
	ExpiresIn string `json:"expiresIn"`
	Status    string `json:"status"`
	SerialNo  string `json:"serialNo"`
}

type ovpn struct {
	address string
}

var (
	version = "1.0.0"
	//go:embed templates
	FS embed.FS

	db     *gorm.DB
	logger = gLogger.New(
		log.New(os.Stdout, "[OPENVPN-WEB] "+time.Now().Format("2006-01-02 15:04:05.000")+" MAIN ", 0),
		gLogger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  gLogger.Error,
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		},
	)
	ovData = os.Getenv("OVPN_DATA")
)

func (ov *ovpn) sendCommand(command string) (string, error) {
	var data string
	var sb strings.Builder

	conn, err := net.DialTimeout("tcp", ov.address, time.Second*10)
	if err != nil {
		logger.Error(context.Background(), err.Error())
		return data, err
	}

	defer conn.Close()

	conn.SetDeadline(time.Now().Add(time.Second * 10))
	conn.Write([]byte(fmt.Sprintf("%s\n", command)))

	for {
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)

		re := regexp.MustCompile(">INFO(.)*\r\n")
		if str := re.ReplaceAllString(string(buf[:n]), ""); str != "" {
			sb.Write([]byte(str))
		}

		if err != nil || strings.HasSuffix(sb.String(), "\r\nEND\r\n") || strings.HasPrefix(sb.String(), "SUCCESS:") {
			break
		}
	}

	data = strings.TrimPrefix(strings.TrimSuffix(strings.TrimSuffix(sb.String(), "\r\nEND\r\n"), "\r\n"), "SUCCESS: ")

	return data, nil
}

func (ov *ovpn) getClient() []ClientData {
	clients := make([]ClientData, 0)

	data, err := ov.sendCommand("status 3")
	if err != nil {
		return clients
	}

	for _, v := range strings.Split(data, "\r\n") {
		cdSlice := strings.Split(v, "\t")

		if cdSlice[0] == "CLIENT_LIST" {
			recv, _ := strconv.ParseFloat(cdSlice[5], 64)
			send, _ := strconv.ParseFloat(cdSlice[6], 64)
			connDate, _ := time.ParseInLocation("2006-01-02 15:04:05", cdSlice[7], time.Local)

			rip := cdSlice[2]
			if strings.Count(cdSlice[2], ":") == 1 {
				rip = cdSlice[2][:strings.IndexByte(cdSlice[2], ':')]
			}

			username := cdSlice[9]
			if username == "UNDEF" {
				username = cdSlice[1]
			}

			cd := ClientData{
				Rip:        rip,
				Vip:        cdSlice[3],
				Vip6:       cdSlice[4],
				RecvBytes:  tools.FormatBytes(recv),
				SendBytes:  tools.FormatBytes(send),
				ConnDate:   cdSlice[7],
				UserName:   username,
				ID:         cdSlice[10],
				OnlineTime: (time.Duration(time.Now().Unix()-connDate.Unix()) * time.Second).String(),
			}

			clients = append(clients, cd)
		}
	}

	return clients

}

func (ov *ovpn) getServer() ServerData {
	var sd ServerData

	data, err := ov.sendCommand("state")
	if err != nil {
		return sd
	}

	sateSlice := strings.Split(data, ",")
	if len(sateSlice) >= 3 {
		runDate, _ := strconv.ParseInt(sateSlice[0], 10, 64)
		sd.RunDate = time.Unix(runDate, 0).Format("2006-01-02 15:04:05")
		sd.Status = sateSlice[1]
		sd.StatusDesc = sateSlice[2]
		sd.Address = sateSlice[3]
	}

	data, err = ov.sendCommand("load-stats")
	if err != nil {
		return sd
	}

	statsSlice := strings.Split(data, ",")
	for _, v := range statsSlice {
		statsKeySlice := strings.Split(v, "=")

		switch statsKeySlice[0] {
		case "nclients":
			sd.Nclients = statsKeySlice[1]
		case "bytesin":
			in, _ := strconv.ParseFloat(statsKeySlice[1], 64)
			sd.BytesIn = tools.FormatBytes(in)
		case "bytesout":
			out, _ := strconv.ParseFloat(statsKeySlice[1], 64)
			sd.BytesOut = tools.FormatBytes(out)
		}
	}

	data, err = ov.sendCommand("version")
	if err != nil {
		return sd
	}

	for _, v := range strings.Split(data, "\n") {
		if strings.HasPrefix(v, "OpenVPN Version: ") {
			sd.Version = strings.TrimPrefix(v, "OpenVPN Version: ")
		}
	}

	return sd

}

func (ov *ovpn) killClient(cid string) {
	ov.sendCommand(fmt.Sprintf("client-kill %s HALT", cid))
}

func parseCrl(crlPath string) (*CertData, error) {
	crlData, err := os.ReadFile(crlPath)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(crlData)
	if block == nil {
		return nil, fmt.Errorf("无法解析证书文件")
	}

	crl, err := x509.ParseRevocationList(block.Bytes)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	expiresIn := crl.NextUpdate.Sub(now)

	var status string
	var expiresInStr string

	if now.After(crl.NextUpdate) {
		status = "已过期"
		expiresInStr = fmt.Sprintf("已过期 %d 天", int(now.Sub(crl.NextUpdate).Hours()/24))
	} else if expiresIn < 30*24*time.Hour {
		status = "即将过期"
		expiresInStr = fmt.Sprintf("%d 天后过期", int(expiresIn.Hours()/24))
	} else {
		status = "正常"
		expiresInStr = fmt.Sprintf("%d 天后过期", int(expiresIn.Hours()/24))
	}

	return &CertData{
		Name:      strings.TrimSuffix(filepath.Base(crlPath), filepath.Ext(crlPath)),
		Type:      "CRL证书",
		Subject:   "",
		Issuer:    crl.Issuer.String(),
		NotBefore: crl.ThisUpdate.Local().Format("2006-01-02 15:04:05"),
		NotAfter:  crl.NextUpdate.Local().Format("2006-01-02 15:04:05"),
		ExpiresIn: expiresInStr,
		Status:    status,
		SerialNo:  "",
	}, nil
}

func parseCert(certPath string) (*CertData, error) {
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		return nil, fmt.Errorf("无法解析证书文件")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	expiresIn := cert.NotAfter.Sub(now)

	var status string
	var expiresInStr string

	if now.After(cert.NotAfter) {
		status = "已过期"
		expiresInStr = fmt.Sprintf("已过期 %d 天", int(now.Sub(cert.NotAfter).Hours()/24))
	} else if expiresIn < 30*24*time.Hour {
		status = "即将过期"
		expiresInStr = fmt.Sprintf("%d 天后过期", int(expiresIn.Hours()/24))
	} else {
		status = "正常"
		expiresInStr = fmt.Sprintf("%d 天后过期", int(expiresIn.Hours()/24))
	}

	certType := "客户端证书"
	if cert.IsCA {
		certType = "CA证书"
	} else if strings.Contains(cert.Subject.CommonName, "server") {
		certType = "服务器证书"
	}

	return &CertData{
		Name:      strings.TrimSuffix(filepath.Base(certPath), filepath.Ext(certPath)),
		Type:      certType,
		Subject:   cert.Subject.String(),
		Issuer:    cert.Issuer.String(),
		NotBefore: cert.NotBefore.Local().Format("2006-01-02 15:04:05"),
		NotAfter:  cert.NotAfter.Local().Format("2006-01-02 15:04:05"),
		ExpiresIn: expiresInStr,
		Status:    status,
		SerialNo:  cert.SerialNumber.String(),
	}, nil
}

func getCerts(ovData string) []CertData {
	cers := make([]CertData, 0)
	pkiDir := filepath.Join(ovData, "pki")

	caPath := filepath.Join(pkiDir, "ca.crt")
	if cert, err := parseCert(caPath); err == nil {
		cers = append(cers, *cert)
	} else {
		logger.Error(context.Background(), err.Error())
	}

	crlPath := filepath.Join(pkiDir, "crl.pem")
	if cert, err := parseCrl(crlPath); err == nil {
		cers = append(cers, *cert)
	} else {
		logger.Error(context.Background(), err.Error())
	}

	issuedDir := filepath.Join(pkiDir, "issued")
	if files, err := os.ReadDir(issuedDir); err == nil {
		for _, file := range files {
			if strings.HasSuffix(file.Name(), ".crt") {
				certPath := filepath.Join(issuedDir, file.Name())
				if cert, err := parseCert(certPath); err == nil {
					cers = append(cers, *cert)
				} else {
					logger.Error(context.Background(), err.Error())
				}
			}
		}
	} else {
		logger.Error(context.Background(), err.Error())
	}

	return cers
}

func isValidPassword(pw string) bool {
	lower := regexp.MustCompile(`[a-z]`)
	upper := regexp.MustCompile(`[A-Z]`)
	digit := regexp.MustCompile(`[0-9]`)
	special := regexp.MustCompile(`[!@#\$%\^&\*()_+\-=\[\]{};':"\\|,.<>\/?]`)

	count := 0
	if len(pw) >= 12 {
		count++
	}
	if lower.MatchString(pw) {
		count++
	}
	if upper.MatchString(pw) {
		count++
	}
	if digit.MatchString(pw) {
		count++
	}
	if special.MatchString(pw) {
		count++
	}

	return count == 5
}

func genRandomString(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[r.Intn(len(charset))]
	}
	return string(result)
}

func AuthMiddleWare() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		user := session.Get("user")

		if c.Request.URL.Path == "/ovpn/login" || c.Request.URL.Path == "/ovpn/history" {
			if c.ClientIP() == "127.0.0.1" || c.ClientIP() == "::1" {
				c.Next()
				return
			}
		}

		if user == nil {
			c.Redirect(302, "/login")
			c.Abort()
			return
		}

		if user, ok := user.(string); ok {
			if c.Request.URL.Path != "/" && !strings.HasPrefix(c.Request.URL.Path, "/client") && user != adminUsername {
				c.Redirect(302, "/")
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

func init() {
	initConfig()
	loadConfig()
}

func main() {
	ov := ovpn{
		address: ovManage,
	}

	var err error
	db, err = gorm.Open(sqlite.Open(path.Join(ovData, "ovpn.db")), &gorm.Config{
		Logger: logger,
	})

	if err != nil {
		panic(err)
	}

	store := gormsessions.NewStore(db, true, []byte(secretKey))
	cc := cache.New(5*time.Minute, 10*time.Minute)

	db.AutoMigrate(&User{}, &History{})

	r := gin.New()
	r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {

		var statusColor, methodColor, resetColor string
		if param.IsOutputColor() {
			statusColor = param.StatusCodeColor()
			methodColor = param.MethodColor()
			resetColor = param.ResetColor()
		}

		if param.Latency > time.Minute {
			param.Latency = param.Latency.Truncate(time.Second)
		}
		return fmt.Sprintf("[OPENVPN-WEB] %v GIN |%s %3d %s| %13v | %15s |%s %-7s %s %#v\n%s",
			param.TimeStamp.Format("2006-01-02 15:04:05.000"),
			statusColor, param.StatusCode, resetColor,
			param.Latency,
			param.ClientIP,
			methodColor, param.Method, resetColor,
			param.Path,
			param.ErrorMessage,
		)
	}))

	r.Use(sessions.Sessions("user_session", store))

	// r.Use(gin.Recovery())

	templ := template.Must(template.New("").ParseFS(FS, "templates/*.html"))
	r.SetHTMLTemplate(templ)
	f, _ := fs.Sub(FS, "templates/static")
	r.StaticFS("/static", http.FS(f))

	r.GET("/login", func(c *gin.Context) {
		c.HTML(http.StatusOK, "login.html", gin.H{})
	})

	r.POST("/login", func(c *gin.Context) {
		var err error

		session := sessions.Default(c)
		remember7d := c.PostForm("remember7d")

		if remember7d == "on" {
			session.Options(sessions.Options{
				MaxAge: 3600 * 24 * 7,
			})
		} else {
			session.Options(sessions.Options{
				MaxAge: 3600 * 1,
			})
		}

		var u User
		c.ShouldBind(&u)

		if u.Username == adminUsername {
			dp, _ := aes.AesDecrypt(adminPassword, secretKey)
			if u.Password == dp {
				session.Set("user", u.Username)
				session.Save()

				c.JSON(200, gin.H{"message": "登录成功", "redirect": "/admin"})
				return
			} else {
				err = fmt.Errorf("密码错误")
			}
		} else {
			passcode := c.PostForm("passcode")

			if passcode != "" {
				if validUser, ok := cc.Get("valid_user"); ok {
					if u.Username == validUser.(string) {
						if ValidateMfa(passcode, u.Info().MfaSecret) {
							cc.Delete("valid_user")
							session.Set("user", u.Username)
							session.Save()
							c.JSON(200, gin.H{"message": "登录成功", "redirect": "/"})
						} else {
							c.JSON(401, gin.H{"message": "MFA验证失败"})
						}

						return
					}
				}

				c.JSON(401, gin.H{"message": "登录超时", "redirect": "/login"})
				return
			}

			if err = u.Login(false); err == nil {
				if u.Info().MfaSecret != "" {
					cc.Set("valid_user", u.Username, 1*time.Minute)
					c.JSON(200, gin.H{"message": "需要MFA验证"})
					return
				}

				session.Set("user", u.Username)
				session.Save()

				c.JSON(200, gin.H{"message": "登录成功", "redirect": "/"})
				return
			}
		}

		c.JSON(401, gin.H{"message": err.Error()})
	})

	r.GET("/logout", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Clear()
		session.Options(sessions.Options{MaxAge: -1})
		session.Save()
		c.Redirect(302, "/login")
	})

	r.Use(AuthMiddleWare())

	r.GET("/", func(c *gin.Context) {
		session := sessions.Default(c)
		if user, ok := session.Get("user").(string); ok {
			if user == adminUsername {
				c.Redirect(302, "/admin")
				return
			}
		}

		c.HTML(http.StatusOK, "client.html", getConfig().Client)
	})

	r.GET("/admin", func(c *gin.Context) {
		session := sessions.Default(c)
		if user, ok := session.Get("user").(string); ok {
			if user != adminUsername {
				c.Redirect(302, "/")
				return
			}
		}

		c.HTML(http.StatusOK, "index.html", gin.H{
			"server":   ov.getServer(),
			"sysUser":  adminUsername,
			"ldapAuth": ldapAuth,
			"version":  "v" + version,
		})
	})

	r.GET("/settings", func(c *gin.Context) {
		var conf config
		viper.Unmarshal(&conf)

		c.JSON(http.StatusOK, conf)
	})

	r.POST("/settings", func(c *gin.Context) {
		c.Request.ParseForm()
		for k, vs := range c.Request.PostForm {
			val := vs[0]

			switch k {
			case "system.base.admin_password":
				val, _ = aes.AesEncrypt(val, secretKey)
			case "openvpn.ovpn_subnet", "openvpn.ovpn_subnet6":
				_, _, err := net.ParseCIDR(val)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}
			case "openvpn.ovpn_push_dns1", "openvpn.ovpn_push_dns2":
				if net.ParseIP(val) == nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": "invalid IP address: " + val})
					return
				}
			}

			switch val {
			case "true":
				viper.Set(k, true)
			case "false":
				viper.Set(k, false)
			default:
				viper.Set(k, val)
			}
		}
		if err := viper.WriteConfig(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
	})

	ovpn := r.Group("/ovpn")
	{
		ovpn.StaticFS("/download", http.Dir("clients"))

		ovpn.POST("/server", func(c *gin.Context) {
			a := c.PostForm("action")

			switch a {
			case "settings":
				k := c.PostForm("key")
				v := c.PostForm("value")

				if k == "auth-user" {
					msg := "停用"
					if v == "true" {
						msg = "启用"
					}
					cmd := exec.Command("sh", "-c", fmt.Sprintf("/usr/bin/docker-entrypoint.sh auth %s", v))
					if out, err := cmd.CombinedOutput(); err != nil {
						if len(out) == 0 {
							out = []byte(err.Error())
						}
						logger.Error(context.Background(), string(out))
						c.JSON(http.StatusInternalServerError, gin.H{"message": fmt.Sprintf("%s用户认证失败", msg)})
					} else {
						ov.sendCommand("signal SIGHUP")
						c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("%s用户认证成功", msg)})
					}
				}
			case "renewCert":
				day := c.PostForm("day")

				cmd := exec.Command("sh", "-c", fmt.Sprintf("/usr/bin/docker-entrypoint.sh renewcert %v", day))
				if out, err := cmd.CombinedOutput(); err != nil {
					if len(out) == 0 {
						out = []byte(err.Error())
					}
					logger.Error(context.Background(), string(out))
					c.JSON(http.StatusInternalServerError, gin.H{"message": "更新证书失败"})
					return
				}

				ov.sendCommand("signal SIGHUP")
				c.JSON(http.StatusOK, gin.H{"message": "更新证书成功"})
			case "restartSrv":
				_, err := ov.sendCommand("signal SIGHUP")
				if err != nil {
					logger.Error(context.Background(), err.Error())
					c.JSON(http.StatusInternalServerError, gin.H{"message": "重启服务失败"})
					return
				}

				c.JSON(http.StatusOK, gin.H{"message": "重启服务成功"})
			case "getConfig":
				data, err := os.ReadFile(path.Join(ovData, "server.conf"))
				if err != nil {
					logger.Error(context.Background(), err.Error())
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}

				c.JSON(http.StatusOK, gin.H{"content": string(data)})
			case "updateConfig":
				content := c.PostForm("content")

				file, err := os.OpenFile(path.Join(ovData, "server.conf"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
				if err != nil {
					logger.Error(context.Background(), err.Error())
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}
				defer file.Close()

				_, err = file.WriteString(content)
				if err != nil {
					logger.Error(context.Background(), err.Error())
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}

				c.JSON(http.StatusOK, gin.H{"message": "配置更新成功"})
			default:
				c.JSON(http.StatusUnprocessableEntity, gin.H{"message": "未知操作"})
			}

		})

		ovpn.POST("/kill", func(c *gin.Context) {
			cid := c.PostForm("cid")
			ov.killClient(cid)
			c.JSON(http.StatusOK, gin.H{"code": http.StatusOK})
		})

		ovpn.POST("/login", func(c *gin.Context) {
			var u User
			c.ShouldBind(&u)
			err := u.Login(true)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"message": err.Error()})
			} else {
				c.JSON(http.StatusOK, gin.H{"message": "登录成功"})
			}
		})

		ovpn.GET("/online-client", func(c *gin.Context) {
			c.JSON(http.StatusOK, ov.getClient())
		})

		ovpn.GET("/user", func(c *gin.Context) {
			var auth bool
			var u User

			cmd := exec.Command("egrep", "^auth-user-pass-verify", path.Join(ovData, "server.conf"))
			if err := cmd.Run(); err != nil {
				auth = false
			} else {
				auth = true
			}

			c.JSON(http.StatusOK, gin.H{"users": u.All(), "authUser": auth})
		})

		ovpn.POST("/user", func(c *gin.Context) {
			var u User
			c.ShouldBind(&u)

			file, err := c.FormFile("file")
			if err != nil {
				if strings.Contains(err.Error(), "no such file") {
					c.JSON(http.StatusInternalServerError, gin.H{"message": "没有上传文件"})
					return
				}
			} else {
				f, _ := file.Open()

				defer f.Close()

				reader := csv.NewReader(f)

				header, err := reader.Read()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}

				if len(header) != 7 {
					c.JSON(http.StatusInternalServerError, gin.H{"message": "导入文件格式错误"})
					return
				}

				for {
					record, err := reader.Read()
					if err == io.EOF {
						break
					}

					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
						return
					}

					enable := record[3] == "1"
					u := User{
						Username:   record[0],
						Password:   record[1],
						Name:       record[2],
						IsEnable:   &enable,
						ExpireDate: strings.Replace(record[4], "/", " ", 1),
						IpAddr:     record[5],
						OvpnConfig: record[6],
					}

					err = u.Create()
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
						return
					}
				}

				c.JSON(http.StatusOK, gin.H{"message": "导入用户成功"})
				return
			}

			err = u.Create()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			} else {
				c.JSON(http.StatusOK, gin.H{"message": "添加用户成功"})
			}
		})

		ovpn.PATCH("/user", func(c *gin.Context) {
			var u User
			c.ShouldBind(&u)

			if ipAddr, ok := c.Request.PostForm["ipAddr"]; ok {
				if ipAddr[0] == "" {
					db.Model(&u).Update("ip_addr", nil)
				}
			}

			if expireDate, ok := c.Request.PostForm["expireDate"]; ok {
				if expireDate[0] == "" {
					db.Model(&u).Update("expire_date", nil)
				}
			}

			err := u.Update()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			} else {
				c.JSON(http.StatusOK, gin.H{"message": "用户更新成功"})
			}
		})

		ovpn.DELETE("/user/:id", func(c *gin.Context) {
			var u User
			id := c.Param("id")

			err := u.Delete(id)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			} else {
				c.JSON(http.StatusOK, gin.H{"message": "删除用户成功"})
			}
		})

		ovpn.GET("/client", func(c *gin.Context) {
			a := c.Query("a")
			switch a {
			case "getConfig":
				f := c.Query("file")

				data, err := os.ReadFile(path.Join(ovData, f))
				if err != nil {
					if strings.Contains(f, "ccd") && os.IsNotExist(err) {
						c.JSON(http.StatusOK, gin.H{"content": ""})
					} else {
						c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					}
					return
				}

				c.JSON(http.StatusOK, gin.H{"content": string(data)})

				return
			default:
				ccd := make([]ClientConfigData, 0)

				files, _ := os.ReadDir(path.Join(ovData, "clients"))
				for _, file := range files {
					finfo, _ := file.Info()

					f := ClientConfigData{
						Name:     strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())),
						FullName: file.Name(),
						File:     fmt.Sprintf("/ovpn/download/%s", file.Name()),
						Date:     finfo.ModTime().Local().Format("2006-01-02 15:04:05"),
					}
					ccd = append(ccd, f)
				}

				sort.Slice(ccd, func(i, j int) bool {
					return ccd[i].Date < ccd[j].Date
				})

				c.JSON(http.StatusOK, ccd)
			}
		})

		ovpn.PUT("/client", func(c *gin.Context) {
			f := c.Query("file")
			content := c.PostForm("content")
			msg := "客户端更新成功"

			dir := filepath.Dir(path.Join(ovData, f))
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				err := os.MkdirAll(dir, 0755)
				if err != nil {
					logger.Error(context.Background(), err.Error())
				}
			}

			if strings.Contains(f, "ccd") {
				grepCmd := exec.Command("grep", "-q", "^client-config-dir", path.Join(ovData, "server.conf"))
				err := grepCmd.Run()
				if err != nil {
					cmd := exec.Command("sh", "-c", fmt.Sprintf("echo 'client-config-dir %[1]s/ccd' >> %[1]s/server.conf", ovData))
					if out, err := cmd.CombinedOutput(); err != nil {
						if len(out) == 0 {
							out = []byte(err.Error())
						}
						logger.Error(context.Background(), string(out))
					}

					msg += "（未启用CCD需要重启服务）"
				}
			}

			file, err := os.OpenFile(path.Join(ovData, f), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}
			defer file.Close()

			_, err = file.WriteString(content)
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": msg})
		})

		ovpn.POST("/client", func(c *gin.Context) {
			name := c.PostForm("name")
			serverAddr := c.PostForm("serverAddr")
			serverPort := c.PostForm("serverPort")
			config := c.PostForm("config")
			ccdConfig := c.PostForm("ccdConfig")
			mfa := c.PostForm("mfa")

			_, err := os.Stat(path.Join(ovData, "clients", fmt.Sprintf("%s.ovpn", name)))
			if err != nil {
				cmd := exec.Command("sh", "-c", fmt.Sprintf("/usr/bin/docker-entrypoint.sh genclient %#v %#v %#v %#v %#v %#v", name, serverAddr, serverPort, config, ccdConfig, mfa))
				if out, err := cmd.CombinedOutput(); err != nil {
					if len(out) == 0 {
						out = []byte(err.Error())
					}
					logger.Error(context.Background(), string(out))
					c.JSON(http.StatusInternalServerError, gin.H{"message": "客户端添加失败"})
					return
				}
			} else {
				c.JSON(http.StatusUnprocessableEntity, gin.H{"message": "客户端已存在"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "客户端添加成功"})
		})

		ovpn.DELETE("/client/:name", func(c *gin.Context) {
			name := c.Param("name")

			cmd := exec.Command("sh", "-c", fmt.Sprintf("easyrsa --batch revoke %s && easyrsa gen-crl", name))
			if out, err := cmd.CombinedOutput(); err != nil {
				if len(out) == 0 {
					out = []byte(err.Error())
				}
				logger.Error(context.Background(), string(out))
				c.JSON(http.StatusInternalServerError, gin.H{"message": "删除客户端失败"})
				return
			}

			os.Remove(path.Join(ovData, "/clients", fmt.Sprintf("%s.ovpn", name)))
			os.Remove(path.Join(ovData, "/ccd", name))

			c.JSON(http.StatusOK, gin.H{"message": "删除客户端成功"})
		})

		ovpn.GET("/history", func(c *gin.Context) {
			var h History
			var p Params

			c.ShouldBindQuery(&p)

			c.JSON(http.StatusOK, h.Query(p))
		})

		ovpn.POST("/history", func(c *gin.Context) {
			var h History
			c.ShouldBind(&h)

			err := h.Create()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			} else {
				c.JSON(http.StatusOK, gin.H{"message": "添加记录成功"})
			}
		})

		ovpn.GET("/certs", func(c *gin.Context) {
			c.JSON(http.StatusOK, getCerts(ovData))
		})
	}

	client := r.Group("/client")
	{
		client.GET("/userinfo", func(c *gin.Context) {
			var u User

			session := sessions.Default(c)
			if user, ok := session.Get("user").(string); ok {
				u.Username = user
			}

			if ldapAuth {
				l, err := InitLdap()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}

				lu, err := l.Get(u.Username)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}

				c.JSON(http.StatusOK, lu)
				return
			}

			c.JSON(http.StatusOK, u.Info())
		})

		client.POST("/modifyPass", func(c *gin.Context) {
			var u User
			c.ShouldBind(&u)

			if !isValidPassword(u.Password) {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "密码不满足要求（长度12位，包含大小写字母、数字、特殊字符）"})
				return
			}

			if currentPass, ok := c.Request.PostForm["currentPass"]; ok {
				if u.Info().Password != currentPass[0] {
					c.JSON(http.StatusUnauthorized, gin.H{"message": "当前密码错误"})
					return
				}
			}

			err := u.UpdatePassword()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			} else {
				c.JSON(http.StatusOK, gin.H{"message": "密码修改成功"})
			}
		})

		client.GET("/userConfig", func(c *gin.Context) {
			var u User
			session := sessions.Default(c)
			if user, ok := session.Get("user").(string); ok {
				u.Username = user
			}

			u = u.Info()
			configName := u.OvpnConfig

			if ldapAuth {
				l, err := InitLdap()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}

				lu, err := l.Get(u.Username)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
					return
				}

				configName = lu.OvpnConfig
			}

			configFile := path.Join(ovData, "clients", configName)
			hasAuth := func() bool {
				file, err := os.Open(path.Join(ovData, "server.conf"))
				if err != nil {
					return false
				}
				defer file.Close()

				scanner := bufio.NewScanner(file)
				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())
					if strings.HasPrefix(line, "auth-user-pass-verify") {
						return true
					}
				}
				if err := scanner.Err(); err != nil {
					return false
				}
				return false
			}

			if configName == "" {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "该账号未指定配置文件，请联系管理员"})
				return
			}

			data, err := os.ReadFile(configFile)
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": "读取配置文件失败"})
				return
			}

			challengeLine := `static-challenge "Enter MFA code" 1`
			content := string(data)

			if u.MfaSecret != "" {
				if !strings.Contains(content, challengeLine) {
					if !strings.HasSuffix(content, "\n") {
						content += "\n"
					}
					content += challengeLine + "\n"
				}
			} else {
				content = strings.ReplaceAll(content, challengeLine+"\n", "")
			}

			if hasAuth() {
				if strings.Contains(content, "#auth-user-pass") {
					content = strings.ReplaceAll(content, "#auth-user-pass", "auth-user-pass")
				}
			} else {
				if !strings.Contains(content, "#auth-user-pass") {
					content = strings.ReplaceAll(content, "auth-user-pass", "#auth-user-pass")
				}
			}

			c.JSON(http.StatusOK, gin.H{"filename": configName, "content": content})
		})

		client.GET("/mfa", func(c *gin.Context) {
			if ldapAuth {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "LDAP用户不支持设置MFA"})
				return
			}

			var u User

			session := sessions.Default(c)
			if user, ok := session.Get("user").(string); ok {
				u.Username = user
			}

			u = u.Info()
			if u.MfaSecret == "" {
				secret, err := GenMfa(u.Username)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": fmt.Errorf("MFA: %w", err).Error()})
				} else {
					u.MfaSecret = secret
					c.JSON(http.StatusOK, gin.H{"mfaEnable": false, "user": u})
				}
			} else {
				c.JSON(http.StatusOK, gin.H{"mfaEnable": true, "user": u})
			}
		})

		client.POST("/mfa", func(c *gin.Context) {
			var u User
			c.ShouldBind(&u)

			passcode := c.PostForm("passcode")

			vaild := ValidateMfa(passcode, u.MfaSecret)
			if !vaild {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "验证码错误"})
			} else {
				u.Update()
				c.JSON(http.StatusOK, gin.H{"message": "MFA已启用"})
			}
		})

		client.DELETE("/mfa/:id", func(c *gin.Context) {
			id := c.Param("id")
			db.Model(&User{}).Where("id = ?", id).Update("mfa_secret", nil)

			c.JSON(http.StatusOK, gin.H{"message": "MFA已停用"})
		})
	}

	r.Run(fmt.Sprintf(":%s", webPort))
}
