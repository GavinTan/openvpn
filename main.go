package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

func init() {
	log.SetPrefix(fmt.Sprintf("[OPENVPN-AUTH] %s ", time.Now().Format("2006-01-02 15:04:05.000")))
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Lshortfile)
}

func main() {
	var adminUser, adminPass string

	username := os.Getenv("username")
	password := os.Getenv("password")
	auth_token := os.Getenv("auth_token")

	opvn_data, ok := os.LookupEnv("ovpn_data")
	if !ok {
		opvn_data = "/data"
	}

	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("source %s/.vars && echo '%s' | openssl enc -d -aes-256-cbc -a -pbkdf2 -k $SECRET_KEY", opvn_data, auth_token))
	out, err := cmd.CombinedOutput()
	if err != nil {
		if out == nil {
			out = []byte(err.Error())
		}
		log.Println(strings.TrimSpace(string(out)))
		os.Exit(1)
	} else {
		if out != nil {
			d := strings.Split(strings.TrimSpace(string(out)), ":")
			if len(d) == 2 {
				adminUser = d[0]
				adminPass = d[1]
			}
		}
	}

	lresp, err := http.PostForm(os.Getenv("auth_api"), url.Values{"username": {adminUser}, "password": {adminPass}})
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	defer lresp.Body.Close()
	io.Copy(io.Discard, lresp.Body)

	if lresp.StatusCode != 200 {
		log.Println("Login failed")
		os.Exit(1)
	}
	cookie := lresp.Header.Get("Set-Cookie")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	b := url.Values{}
	b.Add("username", username)
	b.Add("password", password)

	req, err := http.NewRequest("POST", os.Getenv("ovpn_auth_api"), strings.NewReader(b.Encode()))
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", cookie)

	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	var data struct {
		Message string `json:"message"`
	}
	json.Unmarshal(body, &data)

	log.Printf("[%s] %s\n", username, data.Message)
	if resp.StatusCode != 200 {
		os.Exit(1)
	}

	os.Exit(0)
}
