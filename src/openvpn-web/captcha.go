package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/golang/freetype/truetype"
	"github.com/patrickmn/go-cache"
	"github.com/wenlng/go-captcha-assets/bindata/chars"
	"github.com/wenlng/go-captcha-assets/resources/fonts/fzshengsksjw"
	"github.com/wenlng/go-captcha-assets/resources/imagesv2"
	"github.com/wenlng/go-captcha/v2/base/option"
	"github.com/wenlng/go-captcha/v2/click"
)

const loginCaptchaThreshold = 5

var textCapt click.Captcha

type captchaClickPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

func setLoginFail(ip string) int {
	if ip == "" {
		return 0
	}

	key := "lf:" + ip
	if v, ok := cc.Get(key); ok {
		if n, ok := v.(int); ok {
			n++
			cc.Set(key, n, 30*time.Minute)
			return n
		}
	}

	cc.Set(key, 1, 30*time.Minute)

	return 1
}

func getLoginFail(ip string) int {
	if ip == "" {
		return 0
	}

	if v, ok := cc.Get("lf:" + ip); ok {
		if n, ok := v.(int); ok {
			return n
		}
	}

	return 0
}

func resetLoginFail(ip string) {
	if ip == "" {
		return
	}

	cc.Delete("lf:" + ip)
}

func init() {
	builder := click.NewBuilder(
		click.WithRangeLen(option.RangeVal{Min: 4, Max: 6}),
		click.WithRangeVerifyLen(option.RangeVal{Min: 2, Max: 4}),
	)

	fonts, err := fzshengsksjw.GetFont()
	if err != nil {
		log.Fatalln(err)
	}

	imgs, err := imagesv2.GetImages()
	if err != nil {
		log.Fatalln(err)
	}

	builder.SetResources(
		click.WithChars(chars.GetChineseChars()),
		click.WithFonts([]*truetype.Font{fonts}),
		click.WithBackgrounds(imgs),
	)

	textCapt = builder.Make()
}

func getCaptcha() (string, string, string, error) {
	captData, err := textCapt.Generate()
	if err != nil {
		return "", "", "", err
	}

	dotData := captData.GetData()
	if dotData == nil {
		return "", "", "", fmt.Errorf("生成验证码数据失败")
	}

	var mBase64, tBase64 string
	mBase64, err = captData.GetMasterImage().ToBase64()
	if err != nil {
		return "", "", "", err
	}
	tBase64, err = captData.GetThumbImage().ToBase64()
	if err != nil {
		return "", "", "", err
	}

	key := "captcha:" + genRandomString(16)
	cc.Set(key, dotData, cache.DefaultExpiration)

	return key, mBase64, tBase64, nil
}

func checkCaptcha(key, dots string) bool {
	v, ok := cc.Get(key)
	if !ok {
		return false
	}

	cc.Delete(key)

	dotData, ok := v.(map[int]*click.Dot)
	if !ok {
		return false
	}

	var userPoints []captchaClickPoint
	if err := json.Unmarshal([]byte(dots), &userPoints); err != nil {
		return false
	}

	if len(userPoints) != len(dotData) {
		return false
	}

	for i := 0; i < len(dotData); i++ {
		dot, exists := dotData[i]
		if !exists {
			return false
		}
		p := userPoints[i]
		if !click.Validate(p.X, p.Y, dot.X, dot.Y, dot.Width, dot.Height, 5) {
			return false
		}
	}

	return true
}
