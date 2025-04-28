package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gavintan/gopkg/tools"
	"github.com/gin-contrib/sessions"
	gormsessions "github.com/gin-contrib/sessions/gorm"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/joho/godotenv"
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

type ovpn struct {
	address string
}

var (
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

func AuthMiddleWare() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		user := session.Get("user")

		if c.Request.URL.Path == "/ovpn/login" || c.Request.URL.Path == "/ovpn/history" {
			if strings.Contains(c.Request.Host, "127.0.0.1") {
				c.Next()
				return
			}
		}

		if user == nil {
			c.Redirect(302, "/login")
			c.Abort()
			return
		}

		c.Next()
	}
}

func init() {
	godotenv.Load(path.Join(os.Getenv("OVPN_DATA"), ".vars"))
}

func main() {
	ovData, ok := os.LookupEnv("OVPN_DATA")
	if !ok {
		ovData = "/data"
	}

	ovManage, ok := os.LookupEnv("OVPN_MANAGEMENT")
	if !ok {
		ovManage = "127.0.0.1:7505"
	}

	webPort, ok := os.LookupEnv("WEB_PORT")
	if !ok {
		webPort = "8833"
	}

	secretKey, ok := os.LookupEnv("SECRET_KEY")
	if !ok {
		secretKey = "openvpn-web"
	}

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

	db.AutoMigrate(&User{}, &History{}, &SysUser{})

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

	templ := template.Must(template.New("").ParseFS(FS, "templates/*.tmpl"))
	r.SetHTMLTemplate(templ)
	f, _ := fs.Sub(FS, "templates/static")
	r.StaticFS("/static", http.FS(f))

	r.GET("/login", func(c *gin.Context) {
		c.HTML(http.StatusOK, "login.tmpl", gin.H{})
	})

	r.POST("/login", func(c *gin.Context) {
		var u SysUser
		c.ShouldBind(&u)

		remember7d := c.PostForm("remember7d")

		err := u.Login()
		if err == nil {
			session := sessions.Default(c)
			session.Set("user", u.Username)

			if remember7d == "on" {
				session.Options(sessions.Options{
					MaxAge: 3600 * 24 * 7,
				})
			} else {
				session.Options(sessions.Options{
					MaxAge: 3600 * 1,
				})
			}
			session.Save()

			c.JSON(200, gin.H{"message": "登录成功"})
			return
		}

		c.JSON(401, gin.H{"message": "用户名或密码错误"})

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
		c.HTML(http.StatusOK, "index.tmpl", gin.H{
			"server":  ov.getServer(),
			"sysUser": os.Getenv("ADMIN_USERNAME"),
		})
	})

	r.POST("/user", func(c *gin.Context) {
		var u SysUser
		c.ShouldBind(&u)

		err := u.Create()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		} else {
			c.JSON(http.StatusOK, gin.H{"message": "添加用户成功"})
		}
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
						if out == nil {
							out = []byte(err.Error())
						}
						logger.Error(context.Background(), string(out))
						c.JSON(http.StatusInternalServerError, gin.H{"message": fmt.Sprintf("%s用户认证失败", msg)})
					} else {
						ov.sendCommand("signal SIGHUP")
						c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("%s用户认证成功", msg)})
					}
				}
			case "renewcert":
				cmd := exec.Command("sh", "-c", "/usr/bin/docker-entrypoint.sh renewcert")
				if out, err := cmd.CombinedOutput(); err != nil {
					if out == nil {
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
			err := u.Login()
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

			cmd := exec.Command("egrep", "^auth-user-pass-verify", path.Join(os.Getenv("OVPN_DATA"), "server.conf"))
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

			err := u.Create()
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

			if a == "getConfig" {
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
			} else {
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
						if out == nil {
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
			config := c.PostForm("config")
			ccdConfig := c.PostForm("ccdConfig")

			_, err := os.Stat(path.Join(ovData, "clients", fmt.Sprintf("%s.ovpn", name)))
			if err != nil {
				cmd := exec.Command("sh", "-c", fmt.Sprintf("/usr/bin/docker-entrypoint.sh genclient %s %s %#v %#v", name, serverAddr, config, ccdConfig))
				if out, err := cmd.CombinedOutput(); err != nil {
					if out == nil {
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
				if out == nil {
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
	}

	r.Run(fmt.Sprintf(":%s", webPort))
}
