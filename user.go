package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/gavintan/gopkg/aes"
	"gorm.io/gorm"
)

type User struct {
	ID         uint      `gorm:"primarykey" json:"id" form:"id"`
	Username   string    `gorm:"uniqueIndex;column:username" json:"username" form:"username"`
	Password   string    `form:"password" json:"password"`
	IsEnable   *bool     `gorm:"default:true" form:"isEnable" json:"isEnable"`
	Name       string    `json:"name" form:"name"`
	ExpireDate string    `gorm:"default:NULL" json:"expireDate" form:"expireDate"`
	IpAddr     string    `gorm:"uniqueIndex;default:NULL" json:"ipAddr" form:"ipAddr"`
	CreatedAt  time.Time `json:"createdAt,omitempty" form:"createdAt,omitempty"`
	UpdatedAt  time.Time `json:"updatedAt,omitempty" form:"updatedAt,omitempty"`
}

func (u *User) BeforeSave(tx *gorm.DB) (err error) {
	if u.Password != "" {
		ep, _ := aes.AesEncrypt(u.Password, os.Getenv("SECRET_KEY"))
		tx.Statement.SetColumn("Password", ep)
	}

	return
}

func (u *User) AfterFind(tx *gorm.DB) (err error) {
	dp, err := aes.AesDecrypt(u.Password, os.Getenv("SECRET_KEY"))
	if err == nil {
		u.Password = dp
	}

	return
}

func (u *User) All() []User {
	var users []User

	result := db.Model(&User{}).WithContext(context.Background()).Find(&users)
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

func (u *User) Login() error {
	user := u.Username
	pass := u.Password

	if ovpn_ldap_auth := os.Getenv("OVPN_LDAP_AUTH"); ovpn_ldap_auth == "true" {
		l, err := InitLdap()
		if err != nil {
			return err
		}

		return l.Auth(user, pass)
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

		if u.Password != pass {
			return fmt.Errorf("密码错误")
		}

		if u.IpAddr != "" {
			ovData, ok := os.LookupEnv("OVPN_DATA")
			if !ok {
				ovData = "/data"
			}
			os.WriteFile(path.Join(ovData, ".ovip"), []byte(u.IpAddr), 0644)
		}

		return nil
	}
}

func (User) TableName() string {
	return "user"
}
