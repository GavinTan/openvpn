package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────────────────────
// 数据模型
// ─────────────────────────────────────────────────────────────────────────────

// FeishuSyncLog 记录每次飞书同步运行的结果。Full / Incremental 共用一张表。
// 通过 TriggeredBy 区分触发来源（"cron" 或 "admin:<username>"）。
type FeishuSyncLog struct {
	ID             uint       `gorm:"primarykey" json:"id"`
	SyncType       string     `gorm:"size:16" json:"syncType"` // "full" | "incremental"
	Status         string     `gorm:"size:16" json:"status"`   // "running" | "success" | "failed"
	StartedAt      time.Time  `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt,omitempty"`
	TotalEmployees int        `json:"totalEmployees"`
	Created        int        `json:"created"`
	Updated        int        `json:"updated"`
	Disabled       int        `json:"disabled"`
	ErrorsJSON     string     `gorm:"type:text" json:"-"`
	TriggeredBy    string     `gorm:"size:64" json:"triggeredBy"`
}

// TableName 显式指定表名，避免 GORM 默认按结构体名加复数。
func (FeishuSyncLog) TableName() string { return "feishu_sync_log" }

// BeforeCreate 在新行落地前设置 StartedAt 兜底值。
func (l *FeishuSyncLog) BeforeCreate(tx *gorm.DB) (err error) {
	if l.StartedAt.IsZero() {
		l.StartedAt = time.Now()
	}
	if l.SyncType == "" {
		l.SyncType = "full"
	}
	if l.Status == "" {
		l.Status = "running"
	}
	return nil
}

// FeishuDept 是飞书 contact-v3 department 接口返回的部门对象。
type FeishuDept struct {
	OpenDeptID   string `json:"open_department_id"`
	Name         string `json:"name"`
	ParentOpenID string `json:"parent_department_id"`
	LeaderOpenID string `json:"leader_user_id,omitempty"`
	MemberCount  int    `json:"member_count,omitempty"`
	Status       int    `json:"status"` // 0=active；其他值由飞书定义
}

// FeishuUser 是飞书 contact-v3 user 接口返回的用户对象。
// 仅包含同步需要的最少字段；其他字段如 avatar、custom_data 等不使用。
type FeishuUser struct {
	OpenID        string   `json:"open_id"`
	UnionID       string   `json:"union_id,omitempty"`
	UserID        string   `json:"user_id,omitempty"`
	EmployeeID    string   `json:"employee_id,omitempty"`
	Name          string   `json:"name"`
	EnName        string   `json:"en_name,omitempty"`
	Email         string   `json:"email,omitempty"`
	Mobile        string   `json:"mobile,omitempty"`
	DepartmentIDs []string `json:"department_ids,omitempty"`
	Status        int      `json:"status"` // 1=active, 2=inactive, 4=resigned, 5=未激活
}

// ─────────────────────────────────────────────────────────────────────────────
// HTTP 客户端
// ─────────────────────────────────────────────────────────────────────────────

const (
	feishuBaseURL = "https://open.feishu.cn"
	tokenTTL      = 2 * time.Hour // tenant_access_token 官方 TTL
	tokenRefresh  = 5 * time.Minute
	maxRetries    = 3
)

// FeishuClient 是飞书 API 的轻量客户端。仅依赖 stdlib + x/time/rate。
//
// 设计要点：
//   - 单一 http.Client，超时 30s
//   - tenant_access_token 内存缓存，sync.Mutex 保护
//   - 限流器 100 req/min 平均速率（rate.Every(600ms)，burst 10）
//   - 退避重试：HTTP 429 或 code>=500 时指数退避（1s, 2s, 4s），最多 3 次
//   - HTTPS 强制：NewFeishuClient 拒绝任何非 https:// 的 baseURL
type FeishuClient struct {
	baseURL   string
	httpClient *http.Client
	appID     string
	appSecret string

	limiter *rate.Limiter

	tokenMu     sync.Mutex
	token       string
	tokenExpiry time.Time
}

// NewFeishuClient 构造客户端。baseURL 为空时使用默认 https://open.feishu.cn。
// 强制 https 协议；appID/appSecret 不能为空。
func NewFeishuClient(appID, appSecret, baseURL string) (*FeishuClient, error) {
	if appID == "" || appSecret == "" {
		return nil, errors.New("飞书 AppID 和 AppSecret 不能为空")
	}
	if baseURL == "" {
		baseURL = feishuBaseURL
	}
	if !strings.HasPrefix(baseURL, "https://") {
		return nil, errors.New("飞书 baseURL 必须使用 https://")
	}
	return &FeishuClient{
		baseURL:    baseURL,
		appID:      appID,
		appSecret:  appSecret,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		limiter:    rate.NewLimiter(rate.Every(600*time.Millisecond), 10),
	}, nil
}

// ── token 管理 ──────────────────────────────────────────────────────────────

type feishuTokenResp struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	TenantAccessToken string `json:"tenant_access_token"`
	Expire            int    `json:"expire"` // 秒
}

// ensureToken 取出一个有效的 tenant_access_token。线程安全。
func (c *FeishuClient) ensureToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.token != "" && time.Now().Add(tokenRefresh).Before(c.tokenExpiry) {
		return c.token, nil
	}

	body, _ := json.Marshal(map[string]string{
		"app_id":     c.appID,
		"app_secret": c.appSecret,
	})
	req, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/open-apis/auth/v3/tenant_access_token/internal", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求 tenant_access_token 失败: %w", err)
	}
	defer resp.Body.Close()

	var tr feishuTokenResp
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("解析 tenant_access_token 响应失败: %w", err)
	}
	if tr.Code != 0 || tr.TenantAccessToken == "" {
		return "", fmt.Errorf("飞书 token 获取失败 code=%d msg=%s", tr.Code, tr.Msg)
	}

	c.token = tr.TenantAccessToken
	ttl := time.Duration(tr.Expire) * time.Second
	if ttl <= 0 {
		ttl = tokenTTL
	}
	c.tokenExpiry = time.Now().Add(ttl)
	return c.token, nil
}

// ── 通用请求 ────────────────────────────────────────────────────────────────

// feishuAPIResp 飞书 v3 接口的统一信封：code=0 表示成功。
type feishuAPIResp struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data,omitempty"`
}

// doAPI 执行一次带鉴权、带限流、带重试的飞书 API 请求，并把响应 data 字段
// 解码到 out。method 是 GET 或 POST；body 在 method=POST 时序列化。
//
// 重试策略：HTTP 429、或 HTTP 2xx 但 code>=500、或网络错误，指数退避 1s/2s/4s 最多 3 次。
func (c *FeishuClient) doAPI(ctx context.Context, method, path string, body any, out any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("飞书限流等待被取消: %w", err)
	}

	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(buf)
	}

	var lastErr error
	backoff := time.Second
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}

		token, err := c.ensureToken(ctx)
		if err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("HTTP 调用失败: %w", err)
			continue
		}

		// HTTP 429 触发重试
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			lastErr = errors.New("HTTP 429 rate limited")
			continue
		}

		raw, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("读取响应失败: %w", readErr)
			continue
		}

		var ar feishuAPIResp
		if err := json.Unmarshal(raw, &ar); err != nil {
			lastErr = fmt.Errorf("解析响应失败: %w (body=%s)", err, truncateForLog(raw))
			continue
		}

		// 业务错误：code>=500 重试，其他视为终态失败
		if ar.Code != 0 {
			if ar.Code >= 500 {
				lastErr = fmt.Errorf("飞书服务端错误 code=%d msg=%s", ar.Code, ar.Msg)
				continue
			}
			return fmt.Errorf("飞书 API 错误 code=%d msg=%s path=%s", ar.Code, ar.Msg, path)
		}

		if out != nil && len(ar.Data) > 0 {
			if err := json.Unmarshal(ar.Data, out); err != nil {
				return fmt.Errorf("解码 data 字段失败: %w", err)
			}
		}
		return nil
	}

	if lastErr == nil {
		lastErr = errors.New("飞书 API 调用超过最大重试次数")
	}
	return lastErr
}

// truncateForLog 防止错误信息里塞进巨大的响应体。
func truncateForLog(b []byte) string {
	const max = 200
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "..."
}

// ── 业务接口 ────────────────────────────────────────────────────────────────

// listChildDepartmentsResp 是分页接口的通用 data 形状。
type feishuPagedResp struct {
	HasMore   bool            `json:"has_more"`
	PageToken string          `json:"page_token"`
	Items     json.RawMessage `json:"items"`
}

// ListChildDepartments 拉取指定部门下一层子部门（不含递归）。
// 返回值：部门列表、下一页 token（空字符串表示已读完）。
func (c *FeishuClient) ListChildDepartments(ctx context.Context, parentDeptID string, pageSize int, pageToken string) ([]FeishuDept, string, error) {
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 50
	}
	path := fmt.Sprintf("/open-apis/contact/v3/departments/%s/children?page_size=%d&fetch_child=false&user_id_type=open_id",
		parentDeptID, pageSize)
	if pageToken != "" {
		path += "&page_token=" + pageToken
	}

	var p feishuPagedResp
	if err := c.doAPI(ctx, "GET", path, nil, &p); err != nil {
		return nil, "", err
	}
	var depts []FeishuDept
	if len(p.Items) > 0 {
		if err := json.Unmarshal(p.Items, &depts); err != nil {
			return nil, "", fmt.Errorf("解析 departments items 失败: %w", err)
		}
	}
	return depts, p.PageToken, nil
}

// ListUsersInDept 拉取指定部门成员列表（不含子部门成员）。
// 返回值：用户列表、下一页 token。
func (c *FeishuClient) ListUsersInDept(ctx context.Context, deptID string, pageSize int, pageToken string) ([]FeishuUser, string, error) {
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 50
	}
	path := fmt.Sprintf("/open-apis/contact/v3/users/find_by_department?department_id=%s&page_size=%d&user_id_type=open_id",
		deptID, pageSize)
	if pageToken != "" {
		path += "&page_token=" + pageToken
	}

	var p feishuPagedResp
	if err := c.doAPI(ctx, "GET", path, nil, &p); err != nil {
		return nil, "", err
	}
	var users []FeishuUser
	if len(p.Items) > 0 {
		if err := json.Unmarshal(p.Items, &users); err != nil {
			return nil, "", fmt.Errorf("解析 users items 失败: %w", err)
		}
	}
	return users, p.PageToken, nil
}

// GetUser 拉取单个用户的完整资料（含 mobile、email 等字段）。
func (c *FeishuClient) GetUser(ctx context.Context, openID string) (*FeishuUser, error) {
	path := fmt.Sprintf("/open-apis/contact/v3/users/%s?user_id_type=open_id&department_id_type=open_department_id", openID)
	var user FeishuUser
	if err := c.doAPI(ctx, "GET", path, nil, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// TestConnection 仅校验 AppID/AppSecret 是否能拿到 token。
// 给"测试连接"按钮用：失败时给出明确的错误信息。
func (c *FeishuClient) TestConnection(ctx context.Context) error {
	_, err := c.ensureToken(ctx)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// 同步编排（Step 3 占位）
// ─────────────────────────────────────────────────────────────────────────────
// FeishuSyncer 与 RunSync / ReconcileGroups / ReconcileUser / DetectLeavers
// 将在 Step 3 实现。

