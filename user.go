package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/gavintan/gopkg/aes"
	"gorm.io/gorm"
)

type User struct {
	ID        uint      `gorm:"primarykey" json:"id" form:"id"`
	Username  string    `gorm:"column:username" json:"username" form:"username"`
	Password  string    `form:"password" json:"password"`
	IsEnable  *bool     `gorm:"default:true" form:"isEnable" json:"isEnable"`
	Name      string    `json:"name" form:"name"`
	CreatedAt time.Time `json:"createdAt,omitempty" form:"createdAt,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty" form:"updatedAt,omitempty"`
}

func (u User) All() []User {
	var users []User

	result := db.Table(u.TableName()).WithContext(context.Background()).Find(&users)
	if result.Error != nil {
		logger.Error(context.Background(), result.Error.Error())
		return []User{}
	}

	for k, v := range users {
		dp, _ := aes.AesDecrypt(v.Password, os.Getenv("SECRET_KEY"))
		users[k].Password = dp
	}

	return users
}

func (u User) Create() error {
	if u.Username == "" || u.Password == "" {
		return fmt.Errorf("非法请求")
	}

	ep, _ := aes.AesEncrypt(u.Password, os.Getenv("SECRET_KEY"))
	u.Password = ep

	result := db.Table(u.TableName()).WithContext(context.Background()).Create(&u)

	return result.Error
}

func (u User) Update(id string, data User) error {
	if data.Password != "" {
		ep, _ := aes.AesEncrypt(data.Password, os.Getenv("SECRET_KEY"))
		data.Password = ep
	}

	result := db.Table(u.TableName()).WithContext(context.Background()).Where("id = ?", id).Updates(data)
	return result.Error
}

func (u User) Delete(id string) error {
	result := db.Table(u.TableName()).WithContext(context.Background()).Unscoped().Delete(&u, id)
	return result.Error
}

func (u User) Login() error {
	pass := u.Password
	result := db.Table(u.TableName()).WithContext(context.Background()).First(&u, "username = ?", u.Username)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("用户名不存在")
	}

	if !*u.IsEnable {
		return fmt.Errorf("账号已禁用")
	}

	dp, _ := aes.AesDecrypt(u.Password, os.Getenv("SECRET_KEY"))
	if dp != pass {
		return fmt.Errorf("密码错误")
	}

	return nil

}

func (User) TableName() string {
	return "user"
}
