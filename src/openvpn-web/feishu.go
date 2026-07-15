package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"
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

// BeforeCreate 在新行落地前设置兜底默认值。
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

// ─────────────────────────────────────────────────────────────────────────────
// 值类型（把 SDK 的指针模型拍平成普通结构体，让同步引擎不接触 SDK 细节）
// ─────────────────────────────────────────────────────────────────────────────

// FeishuDept 是飞书部门在本系统的本地投影。字段已从 SDK 的 *string/*int 解引用。
type FeishuDept struct {
	OpenDeptID   string // open_department_id
	Name         string
	ParentOpenID string // parent_department_id，根部门为 "0"
	LeaderOpenID string
	MemberCount  int
}

// FeishuUser 是飞书用户在本系统的本地投影。
type FeishuUser struct {
	OpenID        string
	UserID        string // 飞书租户内 user_id
	EmployeeNo    string // 工号
	Name          string
	Email         string
	Mobile        string
	DepartmentIDs []string
}

// ─────────────────────────────────────────────────────────────────────────────
// 客户端（薄封装 larksuite/oapi-sdk-go）
// ─────────────────────────────────────────────────────────────────────────────

// FeishuClient 是飞书 API 的薄封装。token 缓存、重试、错误码处理由官方 SDK 负责。
//
// 只暴露同步需要的三个能力：
//   - WalkDepartments：DFS 遍历部门树
//   - ListUsersInDept：拉取某部门成员（不含子部门）
//   - GetUser：单个用户详情（含 mobile/email）
//   - TestConnection：仅校验 AppID/AppSecret 能换到 token
type FeishuClient struct {
	client *lark.Client
}

// NewFeishuClient 构造客户端。appID/appSecret 不能为空；baseURL 为空走默认飞书域名。
// 强制 https（与安全设计一致）。
func NewFeishuClient(appID, appSecret, baseURL string) (*FeishuClient, error) {
	if appID == "" || appSecret == "" {
		return nil, errors.New("飞书 AppID 和 AppSecret 不能为空")
	}
	if baseURL != "" && !strings.HasPrefix(baseURL, "https://") {
		return nil, errors.New("飞书 baseURL 必须使用 https://")
	}

	opts := []lark.ClientOptionFunc{
		lark.WithEnableTokenCache(true),
	}
	if baseURL != "" {
		opts = append(opts, lark.WithOpenBaseUrl(baseURL))
	}

	c := lark.NewClient(appID, appSecret, opts...)
	return &FeishuClient{client: c}, nil
}

// TestConnection 用获取 access_token 的方式校验凭证是否有效。
// 这里用 Get 部门根节点 "0" 的子部门做一次最小调用：成功即凭证有效。
func (c *FeishuClient) TestConnection(ctx context.Context) error {
	req := larkcontact.NewChildrenDepartmentReqBuilder().
		DepartmentId("0").
		DepartmentIdType("open_department_id").
		UserIdType("open_id").
		PageSize(1).
		Build()
	resp, err := c.client.Contact.Department.Children(ctx, req)
	if err != nil {
		return fmt.Errorf("调用飞书失败: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("飞书返回错误 code=%d msg=%s", resp.Code, resp.Msg)
	}
	return nil
}

// WalkDepartments 以 rootDeptID 为根，递归 DFS 遍历整棵部门树。
// 返回值：扁平化后的所有部门（含 rootDeptID 的直接子部门，不含 rootDeptID 自身）。
//
// 注意：根部门 "0" 不映射为 VPN Group（见 plan：根部门成员归默认组），
// 因此这里不返回 "0" 自身，只返回它下面的真实部门。
func (c *FeishuClient) WalkDepartments(ctx context.Context, rootDeptID string) ([]FeishuDept, error) {
	if rootDeptID == "" {
		rootDeptID = "0"
	}
	var result []FeishuDept
	if err := c.walkDept(ctx, rootDeptID, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *FeishuClient) walkDept(ctx context.Context, deptID string, acc *[]FeishuDept) error {
	var pageToken *string
	for {
		reqBuilder := larkcontact.NewChildrenDepartmentReqBuilder().
			DepartmentId(deptID).
			DepartmentIdType("open_department_id").
			UserIdType("open_id").
			PageSize(50)
		if pageToken != nil {
			reqBuilder = reqBuilder.PageToken(*pageToken)
		}

		resp, err := c.client.Contact.Department.Children(ctx, reqBuilder.Build())
		if err != nil {
			return fmt.Errorf("拉取部门 %s 子级失败: %w", deptID, err)
		}
		if !resp.Success() {
			return fmt.Errorf("拉取部门 %s 子级失败 code=%d msg=%s", deptID, resp.Code, resp.Msg)
		}

		if resp.Data == nil {
			break
		}
		for _, d := range resp.Data.Items {
			dept := FeishuDept{
				OpenDeptID:   ptrStr(d.OpenDepartmentId),
				Name:         ptrStr(d.Name),
				ParentOpenID: ptrStr(d.ParentDepartmentId),
				LeaderOpenID: ptrStr(d.LeaderUserId),
				MemberCount:  ptrInt(d.MemberCount),
			}
			*acc = append(*acc, dept)
			// 递归下一层
			if dept.OpenDeptID != "" {
				if err := c.walkDept(ctx, dept.OpenDeptID, acc); err != nil {
					return err
				}
			}
		}

		if resp.Data.HasMore == nil || !*resp.Data.HasMore || resp.Data.PageToken == nil {
			break
		}
		pageToken = resp.Data.PageToken
	}
	return nil
}

// ListUsersInDept 拉取指定部门的直接成员（不含子部门成员），自动分页。
func (c *FeishuClient) ListUsersInDept(ctx context.Context, deptID string) ([]FeishuUser, error) {
	var pageToken *string
	var users []FeishuUser
	for {
		reqBuilder := larkcontact.NewFindByDepartmentUserReqBuilder().
			DepartmentId(deptID).
			DepartmentIdType("open_department_id").
			UserIdType("open_id").
			PageSize(50)
		if pageToken != nil {
			reqBuilder = reqBuilder.PageToken(*pageToken)
		}

		resp, err := c.client.Contact.User.FindByDepartment(ctx, reqBuilder.Build())
		if err != nil {
			return nil, fmt.Errorf("拉取部门 %s 成员失败: %w", deptID, err)
		}
		if !resp.Success() {
			return nil, fmt.Errorf("拉取部门 %s 成员失败 code=%d msg=%s", deptID, resp.Code, resp.Msg)
		}
		if resp.Data == nil {
			break
		}
		for _, u := range resp.Data.Items {
			users = append(users, FeishuUser{
				OpenID:        ptrStr(u.OpenId),
				UserID:        ptrStr(u.UserId),
				EmployeeNo:    ptrStr(u.EmployeeNo),
				Name:          ptrStr(u.Name),
				Email:         ptrStr(u.Email),
				Mobile:        ptrStr(u.Mobile),
				DepartmentIDs: u.DepartmentIds,
			})
		}

		if resp.Data.HasMore == nil || !*resp.Data.HasMore || resp.Data.PageToken == nil {
			break
		}
		pageToken = resp.Data.PageToken
	}
	return users, nil
}

// GetUser 拉取单个用户详情（含 mobile/email）。给"补全资料"场景用。
func (c *FeishuClient) GetUser(ctx context.Context, openID string) (*FeishuUser, error) {
	req := larkcontact.NewGetUserReqBuilder().
		UserId(openID).
		UserIdType("open_id").
		DepartmentIdType("open_department_id").
		Build()
	resp, err := c.client.Contact.User.Get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("拉取用户 %s 失败: %w", openID, err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("拉取用户 %s 失败 code=%d msg=%s", openID, resp.Code, resp.Msg)
	}
	if resp.Data == nil || resp.Data.User == nil {
		return nil, errors.New("用户数据为空")
	}
	u := resp.Data.User
	return &FeishuUser{
		OpenID:        ptrStr(u.OpenId),
		UserID:        ptrStr(u.UserId),
		EmployeeNo:    ptrStr(u.EmployeeNo),
		Name:          ptrStr(u.Name),
		Email:         ptrStr(u.Email),
		Mobile:        ptrStr(u.Mobile),
		DepartmentIDs: u.DepartmentIds,
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// 小工具：SDK 字段解引用
// ─────────────────────────────────────────────────────────────────────────────

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func ptrInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// ─────────────────────────────────────────────────────────────────────────────
// 同步编排（Step 3 实现）
// ─────────────────────────────────────────────────────────────────────────────
// FeishuSyncer 与 RunSync / ReconcileGroups / ReconcileUser / DetectLeavers
// 将在 Step 3 实现。
