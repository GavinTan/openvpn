package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/gavintan/gopkg/aes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SysUser struct {
	ID        uint      `gorm:"primarykey" json:"id" form:"id"`
	Username  string    `gorm:"uniqueIndex;column:username" json:"username" form:"username"`
	Password  string    `form:"password" json:"password"`
	CreatedAt time.Time `json:"createdAt,omitempty" form:"createdAt,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty" form:"updatedAt,omitempty"`
}

func (u SysUser) Create() error {
	if u.Username == "" || u.Password == "" {
		return fmt.Errorf("非法请求")
	}

	ep, _ := aes.AesEncrypt(u.Password, os.Getenv("SECRET_KEY"))
	u.Password = ep

	result := db.Table(u.TableName()).WithContext(context.Background()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "username"}},
		DoUpdates: clause.AssignmentColumns([]string{"password"}),
	}).Create(&u)

	return result.Error
}

func (u SysUser) Login() error {
	pass := u.Password
	result := db.Table(u.TableName()).WithContext(context.Background()).First(&u, "username = ?", u.Username)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		if u.Username == os.Getenv("ADMIN_USERNAME") && u.Password == os.Getenv("ADMIN_PASSWORD") {
			return nil
		} else {
			return fmt.Errorf("密码错误")
		}
	}

	dp, _ := aes.AesDecrypt(u.Password, os.Getenv("SECRET_KEY"))
	if dp != pass {
		return fmt.Errorf("密码错误")
	}

	return nil

}

func (SysUser) TableName() string {
	return "system_user"
}
