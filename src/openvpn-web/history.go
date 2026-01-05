package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/gavintan/gopkg/tools"
	"github.com/spf13/viper"
)

type History struct {
	ID            uint      `gorm:"primarykey" json:"id" form:"id"`
	Vip           string    `gorm:"column:vip;comment:'VPN IP'" json:"vip" form:"vip"`
	Rip           string    `gorm:"column:rip;comment:'用户 IP'" json:"rip" form:"rip"`
	CommonName    string    `gorm:"column:common_name;comment:'客户端名称'" json:"common_name" form:"common_name"`
	Username      string    `gorm:"column:username;comment:'用户名'" json:"username" form:"username"`
	BytesReceived float64   `gorm:"comment:'下载流量'" form:"bytes_received" json:"bytes_received"`
	BytesSent     float64   `gorm:"comment:'上传流量'" json:"bytes_sent" form:"bytes_sent"`
	TimeUnix      int64     `gorm:"comment:'上线时间'" json:"time_unix" form:"time_unix"`
	TimeDuration  int64     `gorm:"column:time_duration;comment:'在线时长'" json:"time_duration" form:"time_duration"`
	CreatedAt     time.Time `json:"createdAt,omitempty" form:"createdAt,omitempty"`
}

type QueryData struct {
	Draw            int       `json:"draw"`
	RecordsTotal    int64     `json:"recordsTotal"`
	RecordsFiltered int64     `json:"recordsFiltered"`
	Data            []History `json:"data"`
}

func (h History) MarshalJSON() ([]byte, error) {
	type th History

	return json.Marshal(&struct {
		BytesReceived string `json:"bytes_received"`
		BytesSent     string `json:"bytes_sent"`
		TimeUnix      string `json:"time_unix"`
		TimeDuration  string `json:"time_duration"`
		CreatedAt     string `json:"createdAt"`
		th
	}{
		BytesReceived: tools.FormatBytes(h.BytesReceived),
		BytesSent:     tools.FormatBytes(h.BytesSent),
		TimeUnix:      time.Unix(h.TimeUnix, 0).Format("2006-01-02 15:04:05"),
		CreatedAt:     h.CreatedAt.Format("2006-01-02 15:04:05"),
		TimeDuration:  (time.Duration(h.TimeDuration) * time.Second).String(),
		th:            (th)(h),
	})
}

func (h History) All() []History {
	var historyList []History

	result := db.Table(h.TableName()).WithContext(context.Background()).Find(&historyList)
	if result.Error != nil {
		logger.Error(context.Background(), result.Error.Error())
		return []History{}
	}

	return historyList
}

func (h History) Create() error {
	result := db.Table(h.TableName()).WithContext(context.Background()).Create(&h)

	return result.Error
}

func (h History) Delete(id string) error {
	result := db.Table(h.TableName()).WithContext(context.Background()).Unscoped().Delete(&h, id)
	return result.Error
}

func (h History) Query(p Params) QueryData {
	var qd QueryData
	var itmes []History
	var totalCount int64
	var filterCount int64

	db := db.Table(h.TableName())
	qdb := db.WithContext(context.Background())

	db.Count(&totalCount)

	if p.Qt != "" {
		qt := strings.Split(p.Qt, ",")
		qdb = qdb.Where("time_unix BETWEEN ? AND ?", qt[0], qt[1])
		qdb.Count(&totalCount)
	}

	if p.Search != "" {
		qdb = qdb.Where("vip LIKE @value OR rip LIKE @value OR common_name LIKE @value OR username LIKE @value", sql.Named("value", "%"+p.Search+"%"))
		qdb.Count(&filterCount)
	} else {
		filterCount = totalCount
	}

	result := qdb.Order(p.OrderColumn + " " + p.Order).Offset(p.Offset).Limit(p.Limit).Find(&itmes)
	if result.Error != nil {
		logger.Error(context.Background(), result.Error.Error())
		return QueryData{}
	}

	qd.Draw = p.Draw
	qd.Data = itmes
	qd.RecordsTotal = totalCount
	qd.RecordsFiltered = filterCount

	return qd
}

func (h History) Clear() error {
	result := db.Where("created_at < ?", time.Now().AddDate(0, 0, -viper.GetInt("system.base.history_max_days"))).Delete(&h)
	return result.Error
}

func (History) TableName() string {
	return "history"
}
