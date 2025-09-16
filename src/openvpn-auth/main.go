package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

func init() {
	log.SetPrefix(fmt.Sprintf("[OPENVPN-AUTH] %s ", time.Now().Format("2006-01-02 15:04:05.000")))
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Lshortfile)
}

func main() {
	username := os.Getenv("username")
	password := os.Getenv("password")

	resp, err := http.PostForm(os.Getenv("ovpn_auth_api"), url.Values{"username": {username}, "password": {password}})
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
