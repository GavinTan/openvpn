package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gavintan/gopkg/aes"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

type User struct {
	ID         uint      `gorm:"primarykey" json:"id" form:"id"`
	Username   string    `gorm:"uniqueIndex;column:username" json:"username" form:"username"`
	Password   string    `form:"password" json:"password"`
	IsEnable   *bool     `gorm:"default:true" form:"isEnable" json:"isEnable"`
	Name       string    `json:"name" form:"name"`
	Email      string    `json:"email" form:"email"`
	Gid        uint      `gorm:"default:1" json:"gid" form:"gid"`
	ExpireDate string    `gorm:"default:NULL" json:"expireDate" form:"expireDate"`
	IpAddr     string    `gorm:"uniqueIndex;default:NULL" json:"ipAddr" form:"ipAddr"`
	OvpnConfig string    `json:"ovpnConfig" form:"ovpnConfig"`
	MfaSecret  string    `json:"mfaSecret" form:"mfaSecret"`
	CreatedAt  time.Time `json:"createdAt,omitempty" form:"createdAt,omitempty"`
	UpdatedAt  time.Time `json:"updatedAt,omitempty" form:"updatedAt,omitempty"`
}

func (u *User) BeforeSave(tx *gorm.DB) (err error) {
	if u.Password != "" {
		ep, _ := aes.AesEncrypt(u.Password, secretKey)
		tx.Statement.SetColumn("Password", ep)
	}

	return
}

func (u *User) AfterFind(tx *gorm.DB) (err error) {
	dp, err := aes.AesDecrypt(u.Password, secretKey)
	if err == nil {
		u.Password = dp
	}

	return
}

func (u *User) All() []User {
	var users []User

	result := db.WithContext(context.Background()).Find(&users)
	if result.Error != nil {
		logger.Error(context.Background(), result.Error.Error())
		return []User{}
	}

	return users
}

func (u *User) Create() error {
	if u.Username == "" || u.Password == "" {
		return fmt.Errorf("非法请求")
	}

	if u.Username == adminUsername {
		return fmt.Errorf("用户名与系统账户冲突")
	}

	result := db.Create(&u)
	return result.Error
}

func (u *User) Update() error {
	result := db.Model(&u).Updates(&u)
	return result.Error
}

func (u *User) Delete(id string) error {
	result := db.Unscoped().Delete(&User{}, id)
	return result.Error
}

func (u *User) UpdatePassword() error {
	result := db.Model(&u).Updates(User{Password: u.Password})
	return result.Error
}

func (u *User) Login(clogin bool) error {
	user := u.Username
	pass := u.Password
	commonName := u.OvpnConfig

	if clogin {
		if viper.GetInt("system.base.max_duplicate_login") > 0 {
			data, err := os.ReadFile(path.Join(ovData, "openvpn-status.log"))
			if err != nil {
				logger.Error(context.Background(), err.Error())
			}

			loginCount := 0
			for _, v := range strings.Split(string(data), "\n") {
				cdSlice := strings.Split(v, "\t")

				if cdSlice[0] == "CLIENT_LIST" {
					if cdSlice[9] == user {
						loginCount++
					}
				}
			}

			if loginCount >= viper.GetInt("system.base.max_duplicate_login") {
				return fmt.Errorf("用户已登录数量超过限制")
			}
		}
	}

	if ldapAuth {
		l, err := InitLdap()
		if err != nil {
			return err
		}

		return l.Auth(clogin, user, pass, commonName)
	} else {
		result := db.First(&u, "username = ?", user)

		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return fmt.Errorf("用户名不存在")
		}

		if !*u.IsEnable {
			return fmt.Errorf("账号已禁用")
		}

		if u.ExpireDate != "" {
			ed, _ := time.Parse("2006-01-02", u.ExpireDate)
			if ed.Before(time.Now()) {
				return fmt.Errorf("账号已过期")
			}
		}

		if clogin {
			if u.MfaSecret != "" && !strings.HasPrefix(pass, "SCRV1:") {
				return fmt.Errorf("未获取到MFA验证码")
			}

			var passcode string
			if strings.HasPrefix(pass, "SCRV1:") {
				parts := strings.Split(pass, ":")
				if len(parts) == 3 {
					p, err := base64.StdEncoding.DecodeString(parts[1])
					if err != nil {
						return fmt.Errorf("passwd解码错误：%w", err)
					}

					pass = string(p)

					k, err := base64.StdEncoding.DecodeString(parts[2])
					if err != nil {
						return fmt.Errorf("key解码错误：%w", err)
					}

					passcode = string(k)
				}
			}

			if u.MfaSecret != "" {
				vaild := ValidateMfa(passcode, u.MfaSecret)
				if !vaild {
					return fmt.Errorf("MFA验证失败")
				}
			}
		}

		if u.Password != pass {
			return fmt.Errorf("密码错误")
		}

		if clogin {
			if commonName != strings.TrimSuffix(u.OvpnConfig, ".ovpn") {
				return fmt.Errorf("使用非法配置文件登录")
			}

			if u.IpAddr != "" {
				os.WriteFile(path.Join(ovData, ".ovip"), []byte(u.IpAddr), 0644)
			}

			var ovconfig sql.NullString
			db.Raw(`
				WITH RECURSIVE group_up AS (
					SELECT
						id,
						parent_id,
						config,
						0 AS level
					FROM "group"
					WHERE id = ?
			
					UNION ALL
			
					SELECT
						g.id,
						g.parent_id,
						g.config,
						gu.level + 1
					FROM "group" g
					JOIN group_up gu ON g.id = gu.parent_id
				)
				SELECT GROUP_CONCAT(REPLACE(config, '\n', CHAR(10)), CHAR(10)) AS configs
				FROM group_up
				WHERE config IS NOT NULL
			`, u.Gid).Scan(&ovconfig)

			if ovconfig.Valid {
				os.WriteFile(path.Join(ovData, ".ovc"), []byte(ovconfig.String), 0644)
			}
		}

		return nil
	}
}

func (u User) Info() User {
	if u.Username != "" {
		db.First(&u, "username = ?", u.Username)
	} else {
		db.First(&u)
	}

	return u
}

func (User) TableName() string {
	return "user"
}
