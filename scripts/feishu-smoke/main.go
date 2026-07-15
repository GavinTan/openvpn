// 飞书 API 连通性 smoke test（纯标准库，独立 module）。
//
// 用途：用真实 AppID/AppSecret 实际调用飞书三个接口，验证
//   1) 凭证有效（能拿到 tenant_access_token）
//   2) 权限 scope 配全（能拉部门、能拉用户、能看到 mobile/email）
//   3) 真实响应字段结构（供 openvpn-web 端解析核对）
//
// 输出已脱敏：不打印 token 全文、不打印手机号/邮箱内容，只显示字段是否存在及长度。
//
// 运行（PowerShell）：
//   $env:FEISHU_APP_ID="cli_xxx"; $env:FEISHU_APP_SECRET="你的secret"
//   cd scripts\feishu-smoke; go run .
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

func main() {
	appID := os.Getenv("FEISHU_APP_ID")
	appSecret := os.Getenv("FEISHU_APP_SECRET")
	base := os.Getenv("FEISHU_BASE_URL")
	if base == "" {
		base = "https://open.feishu.cn"
	}
	if appID == "" || appSecret == "" {
		fmt.Fprintln(os.Stderr, "缺少环境变量：请设置 FEISHU_APP_ID 和 FEISHU_APP_SECRET")
		os.Exit(1)
	}

	token, err := getToken(base, appID, appSecret)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[FAIL] 获取 tenant_access_token 失败:", err)
		fmt.Fprintln(os.Stderr, "  → 检查 AppID/AppSecret 是否正确、app 是否已创建并发布")
		os.Exit(1)
	}
	fmt.Printf("[OK] tenant_access_token 获取成功 (前缀 %s…, expire 见下)\n\n", mask(token))

	fmt.Println("=== 部门接口 GET /contact/v3/departments/0/children ===")
	getAndDump(base, token, "/open-apis/contact/v3/departments/0/children?page_size=5&user_id_type=open_id&department_id_type=open_department_id")

	fmt.Println("\n=== 用户接口 GET /contact/v3/users/find_by_department?department_id=0 ===")
	getAndDump(base, token, "/open-apis/contact/v3/users/find_by_department?department_id=0&page_size=5&user_id_type=open_id&department_id_type=open_department_id")

	// 取第一个真实子部门，再查它的成员（验证子部门能拉到用户、字段结构）
	firstDept, err := fetchFirstDeptOpenID(base, token, "0")
	if err != nil {
		fmt.Printf("\n[SKIP] 无法获取子部门用于用户查询: %v\n", err)
	} else {
		fmt.Printf("\n=== 用户接口(真实子部门 %s) ===\n", firstDept)
		getAndDump(base, token, "/open-apis/contact/v3/users/find_by_department?department_id="+url.QueryEscape(firstDept)+"&page_size=5&user_id_type=open_id&department_id_type=open_department_id")
	}

	fmt.Println("\n=== 完成 ===")
	fmt.Println("把以上输出贴回，我据此核对 openvpn-web 端的字段解析与权限配置是否正确。")
}

// fetchFirstDeptOpenID 拉取 parentDeptID 的第一个子部门 open_department_id。
func fetchFirstDeptOpenID(base, token, parentDeptID string) (string, error) {
	req, _ := http.NewRequest("GET", base+"/open-apis/contact/v3/departments/"+url.QueryEscape(parentDeptID)+"/children?page_size=1&user_id_type=open_id&department_id_type=open_department_id", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env struct {
		Code int `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Items []struct {
				OpenDepartmentID string `json:"open_department_id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", err
	}
	if env.Code != 0 {
		return "", fmt.Errorf("code=%d msg=%s", env.Code, env.Msg)
	}
	if len(env.Data.Items) == 0 {
		return "", fmt.Errorf("无子部门")
	}
	return env.Data.Items[0].OpenDepartmentID, nil
}

func getToken(base, appID, appSecret string) (string, error) {
	body, _ := json.Marshal(map[string]string{"app_id": appID, "app_secret": appSecret})
	resp, err := http.Post(base+"/open-apis/auth/v3/tenant_access_token/internal", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var r struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	if r.Code != 0 || r.TenantAccessToken == "" {
		return "", fmt.Errorf("code=%d msg=%s", r.Code, r.Msg)
	}
	return r.TenantAccessToken, nil
}

func getAndDump(base, token, path string) {
	req, _ := http.NewRequest("GET", base+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "  请求失败:", err)
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var env struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	json.Unmarshal(raw, &env)
	fmt.Printf("  HTTP %d  code=%d msg=%q\n", resp.StatusCode, env.Code, env.Msg)
	if env.Code != 0 {
		fmt.Printf("  原始响应: %s\n", truncate(raw))
		fmt.Println("  → 若 code 非 0，通常是权限 scope 未配全或 app 未发布。需要勾选：")
		fmt.Println("    contact:user.id:readonly / contact:user.basic_profile:readonly /")
		fmt.Println("    contact:user.employee_id:readonly / contact:department:readonly /")
		fmt.Println("    contact:department.member:readonly")
		return
	}

	var d struct {
		HasMore   bool            `json:"has_more"`
		PageToken string          `json:"page_token"`
		Items     json.RawMessage `json:"items"`
	}
	json.Unmarshal(env.Data, &d)
	fmt.Printf("  has_more=%v  page_token=%q\n", d.HasMore, d.PageToken)

	var items []json.RawMessage
	json.Unmarshal(d.Items, &items)
	fmt.Printf("  items 数量=%d\n", len(items))
	if len(items) > 0 {
		fmt.Println("  第一个 item 字段（脱敏，只显示类型/是否非空）:")
		var first map[string]interface{}
		json.Unmarshal(items[0], &first)
		for k, v := range first {
			fmt.Printf("    %-24s %s\n", k, describe(v))
		}
	}
}

func describe(v interface{}) string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return "空字符串"
		}
		return fmt.Sprintf("有值(长度 %d)", len(t))
	case float64:
		return fmt.Sprintf("数字 %v", t)
	case bool:
		return fmt.Sprintf("布尔 %v", t)
	case []interface{}:
		return fmt.Sprintf("数组(长度 %d)", len(t))
	case map[string]interface{}:
		return fmt.Sprintf("对象(%d 键)", len(t))
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", t)
	}
}

func mask(s string) string {
	if len(s) <= 8 {
		return "***"
	}
	return s[:8]
}

func truncate(b []byte) string {
	if len(b) > 300 {
		return string(b[:300]) + "…"
	}
	return string(b)
}
