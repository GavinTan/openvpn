package main

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
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

	"github.com/gavintan/gopkg/aes"
	"github.com/gavintan/gopkg/tools"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

//go:embed templates
var FS embed.FS

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

type UserData struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	UserName  string         `gorm:"column:username" json:"username"`
	Password  string         `form:"password" json:"password"`
	IsEnable  *bool          `gorm:"default:true" form:"isEnable" json:"isEnable"`
	Name      string         `json:"name"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deletedAt"`
}

type ClientConfigData struct {
	Name     string `json:"name"`
	FullName string `json:"fullName"`
	File     string `json:"file"`
	Date     string `json:"date"`
}

type UserObj struct {
	db *gorm.DB
}

type ovpn struct {
	server string
}

func Error(err interface{}) {
	l := log.New(os.Stdout, fmt.Sprintf("[OPENVPN-WEB] %s ", time.Now().Format("2006-01-02 15:04:05.000")), log.Llongfile)
	l.Printf("\033[31m%s\033[0m", strings.Trim(fmt.Sprintf("%s", err), "\n"))
}

func (ov *ovpn) sendCommand(command string) (string, error) {
	var data string
	var sb strings.Builder

	conn, err := net.DialTimeout("tcp", ov.server, time.Second*10)
	if err != nil {
		Error(err)
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
				OnlineTime: time.Since(connDate).String(),
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
	ov.sendCommand(fmt.Sprintf("client-kill %s", cid))
}

func User() *UserObj {
	gromLogger := logger.New(
		log.New(os.Stdout, "[OPENVPN-WEB] "+time.Now().Format("2006-01-02 15:04:05.000")+" GORM ", 0),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Error,
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		},
	)
	db, err := gorm.Open(sqlite.Open("ovpn.db"), &gorm.Config{
		Logger: gromLogger,
	})

	if err != nil {
		return &UserObj{db: nil}
	}

	db.Table("user").AutoMigrate(&UserData{})

	return &UserObj{db: db.Table("user")}
}

func (u UserObj) All() []UserData {
	var res []UserData

	if u.db != nil {
		u.db.Find(&res)
		defer u.Close()
	}

	for k, v := range res {
		pass, _ := aes.AesDecrypt(v.Password, os.Getenv("SECRET_KEY"))
		res[k].Password = pass
	}

	return res
}

func (u UserObj) Create(data UserData) {
	if u.db != nil {
		if data.Password != "" {
			epassword, _ := aes.AesEncrypt(data.Password, os.Getenv("SECRET_KEY"))
			data.Password = epassword
		}
		u.db.Create(&data)
		defer u.Close()
	}

}

func (u UserObj) Update(id string, data UserData) {
	if u.db != nil {
		if data.Password != "" {
			epassword, _ := aes.AesEncrypt(data.Password, os.Getenv("SECRET_KEY"))
			data.Password = epassword
		}
		u.db.Model(&UserData{}).Where("id = ?", id).Updates(data)
		defer u.Close()
	}
}

func (u UserObj) Delete(id string) {
	if u.db != nil {
		u.db.Unscoped().Delete(&UserData{}, id)
		defer u.Close()
	}
}

func (u UserObj) Login(username string, password string) error {
	var loginUser UserData

	if u.db == nil {
		return fmt.Errorf("读取数据库出错")
	}

	r := u.db.First(&loginUser, "username = ?", username)
	defer u.Close()
	if errors.Is(r.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("用户名不存在")
	}

	if !*loginUser.IsEnable {
		return fmt.Errorf("账号已禁用")
	}

	pass, _ := aes.AesDecrypt(loginUser.Password, os.Getenv("SECRET_KEY"))
	if pass != password {
		return fmt.Errorf("密码错误")
	}

	return nil

}

func (u UserObj) Close() {
	if db, err := u.db.DB(); err == nil {
		db.Close()
	}
}

func init() {
	godotenv.Load(path.Join(os.Getenv("OVPN_DATA"), ".vars"))
}

func main() {
	port, ok := os.LookupEnv("OVPN_MANAGE_PORT")
	if !ok {
		port = "7505"
	}
	ov := ovpn{
		server: fmt.Sprintf(":%s", port),
	}

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

	// r.Use(gin.Recovery())

	templ := template.Must(template.New("").ParseFS(FS, "templates/*.tmpl"))
	r.SetHTMLTemplate(templ)

	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.tmpl", gin.H{
			"server": ov.getServer(),
		})
	})

	r.POST("/server", func(c *gin.Context) {
		a := c.PostForm("action")

		if a == "settings" {
			k := c.PostForm("key")
			if k == "auth-user" {
				restartCmd := "supervisorctl stop openvpn && sleep 2 && supervisorctl start openvpn"
				if v := c.PostForm("value"); v == "true" {
					cmd := exec.Command("sh", "-c", fmt.Sprintf("sed -i 's/^#auth-user-pass-verify/auth-user-pass-verify/' $OVPN_DATA/server.conf && %s", restartCmd))

					if out, err := cmd.CombinedOutput(); err != nil {
						if out == nil {
							out = []byte(err.Error())
						}
						Error(out)
						c.JSON(http.StatusInternalServerError, gin.H{"message": "启用用户认证失败"})
					} else {
						c.JSON(http.StatusOK, gin.H{"message": "启用用户认证成功"})
					}
				} else {
					cmd := exec.Command("sh", "-c", fmt.Sprintf("sed -i 's/^auth-user-pass-verify/#&/' $OVPN_DATA/server.conf && %s", restartCmd))
					if out, err := cmd.CombinedOutput(); err != nil {
						Error(out)
						c.JSON(http.StatusInternalServerError, gin.H{"message": "停用用户认证失败"})
					} else {
						c.JSON(http.StatusOK, gin.H{"message": "停用用户认证成功"})
					}
				}
			}
		}
	})

	r.POST("/kill", func(c *gin.Context) {
		cid := c.PostForm("cid")
		ov.killClient(cid)
		c.JSON(http.StatusOK, gin.H{"code": http.StatusOK})
	})

	r.POST("/login", func(c *gin.Context) {
		username := c.PostForm("username")
		password := c.PostForm("password")

		u := User()
		err := u.Login(username, password)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"message": err.Error()})
		} else {
			c.JSON(http.StatusOK, gin.H{"message": "登录成功"})
		}
	})

	r.GET("/online-client", func(c *gin.Context) {
		c.JSON(http.StatusOK, ov.getClient())
	})

	r.GET("/user", func(c *gin.Context) {
		var auth bool

		cmd := exec.Command("egrep", "^auth-user-pass-verify", path.Join(os.Getenv("OVPN_DATA"), "server.conf"))
		if err := cmd.Run(); err != nil {
			auth = false
		} else {
			auth = true
		}

		c.JSON(http.StatusOK, gin.H{"users": User().All(), "authUser": auth})
	})

	r.POST("/user", func(c *gin.Context) {
		u := User()
		u.Create(UserData{
			Name:     c.PostForm("name"),
			UserName: c.PostForm("username"),
			Password: c.PostForm("password"),
		})

		c.JSON(http.StatusOK, gin.H{"message": "添加用户成功"})
	})

	r.PATCH("/user/:id", func(c *gin.Context) {
		var data UserData
		id := c.Param("id")
		u := User()
		c.Bind(&data)
		u.Update(id, data)

		c.JSON(http.StatusOK, gin.H{"message": "用户更新成功"})
	})

	r.DELETE("/user/:id", func(c *gin.Context) {
		id := c.Param("id")
		u := User()
		u.Delete(id)

		c.JSON(http.StatusOK, gin.H{"message": "删除用户成功"})
	})

	r.GET("/client", func(c *gin.Context) {
		ccd := make([]ClientConfigData, 0)

		files, _ := ioutil.ReadDir("clients")

		for _, file := range files {
			f := ClientConfigData{
				Name:     strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())),
				FullName: file.Name(),
				File:     fmt.Sprintf("/download/%s", file.Name()),
				Date:     file.ModTime().Local().Format("2006-01-02 15:04:05"),
			}
			ccd = append(ccd, f)
		}

		c.JSON(http.StatusOK, ccd)
	})

	r.POST("/client", func(c *gin.Context) {
		name := c.PostForm("name")
		serverAddr := c.PostForm("serverAddr")
		config := c.PostForm("config")

		_, err := os.Stat(path.Join("clients", fmt.Sprintf("%s.ovpn", name)))
		if err != nil {
			cmd := exec.Command("sh", "-c", fmt.Sprintf("/usr/bin/docker-entrypoint.sh genclient %s %s %#v", name, serverAddr, config))
			if out, err := cmd.CombinedOutput(); err != nil {
				if out == nil {
					out = []byte(err.Error())
				}
				Error(out)
				c.JSON(http.StatusInternalServerError, gin.H{"message": "客户端添加失败"})
				return
			}
		} else {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"message": "客户端已存在"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "客户端添加成功"})
	})

	r.DELETE("/client/:name", func(c *gin.Context) {
		name := c.Param("name")

		cmd := exec.Command("sh", "-c", fmt.Sprintf("easyrsa --batch revoke %s && easyrsa gen-crl", name))
		if out, err := cmd.CombinedOutput(); err != nil {
			if out == nil {
				out = []byte(err.Error())
			}
			Error(out)
			c.JSON(http.StatusInternalServerError, gin.H{"message": "删除客户端失败"})
			return
		}

		os.Remove(path.Join("/data/clients", fmt.Sprintf("%s.ovpn", name)))
		c.JSON(http.StatusOK, gin.H{"message": "删除客户端成功"})
	})

	r.StaticFS("/download", http.Dir("clients"))

	r.Run(":80")
}
