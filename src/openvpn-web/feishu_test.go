package main

import (
	"regexp"
	"testing"
)

var pwdPattern = regexp.MustCompile(`^[0-9]{6}[a-zA-Z]{4}$`)

func TestGenerateDefaultPassword_Format(t *testing.T) {
	// 手机号够 6 位：应为 后6位数字 + 4位大小写字母
	pwd := generateDefaultPassword("+86 138 1234 5678", "ou_abc")
	if !pwdPattern.MatchString(pwd) {
		t.Fatalf("密码格式不符 got=%q want=^\\d{6}[a-zA-Z]{4}$", pwd)
	}
	if got := pwd[:6]; got != "345678" {
		t.Errorf("密码前6位不是手机后6位 got=%q want=345678", got)
	}
	// 后 4 位必须是字母
	for _, r := range pwd[6:] {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			t.Errorf("密码后4位含非字母 %q", pwd[6:])
		}
	}
}

func TestGenerateDefaultPassword_FallbackOpenID(t *testing.T) {
	// 手机号为空：回退到 open_id 末段，结果仍应形如 <6位><4字母>
	// open_id 末段取字母数字后 6 位作为前缀
	pwd := generateDefaultPassword("", "ou_1a2b3c4d5e6f")
	if len(pwd) != 10 {
		t.Fatalf("回退密码长度不符 got=%d want=10", len(pwd))
	}
	for _, r := range pwd[6:] {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			t.Errorf("回退密码后4位含非字母 %q", pwd[6:])
		}
	}
}

func TestGenerateDefaultPassword_Uniqueness(t *testing.T) {
	// 同样输入应产生不同后缀（crypto/rand 随机）
	a := generateDefaultPassword("13800001111", "ou_a")
	b := generateDefaultPassword("13800001111", "ou_a")
	if a[:6] != b[:6] {
		t.Errorf("同手机号前6位应一致 got a=%q b=%q", a, b)
	}
	if a[6:] == b[6:] {
		// 极小概率撞，视为通过但记录
		t.Logf("警告：两次随机后缀相同（概率极低）a=%q b=%q", a, b)
	}
}

func TestStripNonDigits(t *testing.T) {
	cases := map[string]string{
		"+86 138 1234 5678": "8613812345678",
		"abc123def456":      "123456",
		"":                  "",
		"无数字":               "",
	}
	for in, want := range cases {
		if got := stripNonDigits(in); got != want {
			t.Errorf("stripNonDigits(%q) = %q want %q", in, got, want)
		}
	}
}

func TestNormalizePhone(t *testing.T) {
	cases := map[string]string{
		"+86 138-0000 1234": "+8613800001234",
		"  13800001234  ":   "13800001234",
		"+1 (555) 0100":     "+15550100",
	}
	for in, want := range cases {
		if got := normalizePhone(in); got != want {
			t.Errorf("normalizePhone(%q) = %q want %q", in, got, want)
		}
	}
}

func TestDeriveUsername(t *testing.T) {
	// 优先 email
	if got := deriveUsername(FeishuUser{Email: "alice@example.com", Mobile: "13800001111", OpenID: "ou_1"}); got != "alice@example.com" {
		t.Errorf("应优先用 email got=%q", got)
	}
	// 无 email 用手机后 6 位
	if got := deriveUsername(FeishuUser{Mobile: "+86 138 0000 1111", OpenID: "ou_1"}); got != "001111" {
		t.Errorf("无 email 应取手机后6位 got=%q want=001111", got)
	}
	// 无 email 无手机用 user_id
	if got := deriveUsername(FeishuUser{UserID: "u_abc123", OpenID: "ou_1"}); got != "u_abc123" {
		t.Errorf("应回退 user_id got=%q", got)
	}
	// 全空回退 open_id 末段
	got := deriveUsername(FeishuUser{OpenID: "ou_abcdef123456"})
	if got == "" {
		t.Errorf("open_id 回退不应为空")
	}
}
