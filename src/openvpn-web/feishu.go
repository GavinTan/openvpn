package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"
	"github.com/spf13/viper"
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
// 同步编排
// ─────────────────────────────────────────────────────────────────────────────

// FeishuSyncConfig 是同步引擎运行时需要的配置。由 config.go（Step 4）从 viper
// 填充后传给 NewFeishuSyncer。这里不直接依赖 viper，保持可测试。
type FeishuSyncConfig struct {
	AppID          string
	AppSecret      string
	BaseURL        string
	RootDeptID     string // 根部门 ID，默认 "0"
	DefaultGroupID uint   // 根部门成员归入的默认 VPN 组 ID
	DisableOnLeave bool   // 离职是否自动禁用本地账号
	NotifyOnCreate bool   // 新建用户是否发送欢迎邮件
}

// ErrSyncAlreadyRunning 表示已有一次同步正在进行，拒绝重叠执行。
var ErrSyncAlreadyRunning = errors.New("飞书同步任务正在执行中，请稍后重试")

// feishuSyncerMu 是跨实例的全局互斥，保证同一进程内同一时刻只有一个同步任务运行
//（cron 与手动触发共享）。
var feishuSyncerMu sync.Mutex

// FeishuSyncer 是同步编排器。一次进程内应复用，但每次 RunSync 都会加锁。
type FeishuSyncer struct {
	cfg    FeishuSyncConfig
	client *FeishuClient
}

// NewFeishuSyncer 构造同步器并建立飞书客户端。
func NewFeishuSyncer(cfg FeishuSyncConfig) (*FeishuSyncer, error) {
	c, err := NewFeishuClient(cfg.AppID, cfg.AppSecret, cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	return &FeishuSyncer{cfg: cfg, client: c}, nil
}

// RunSync 执行一次同步。kind 取值 "full" 或 "incremental"（当前两者实现一致，
// 全量幂等即可覆盖增量场景）。triggeredBy 记录触发来源用于审计。
//
// 返回写入了数据的 FeishuSyncLog 行（无论成功失败都会落盘一行）。
func (s *FeishuSyncer) RunSync(ctx context.Context, kind, triggeredBy string) (*FeishuSyncLog, error) {
	if !feishuSyncerMu.TryLock() {
		return nil, ErrSyncAlreadyRunning
	}
	defer feishuSyncerMu.Unlock()

	log := &FeishuSyncLog{
		SyncType:    kind,
		Status:      "running",
		StartedAt:   time.Now(),
		TriggeredBy: triggeredBy,
	}
	if err := db.Create(log).Error; err != nil {
		return nil, fmt.Errorf("创建同步日志失败: %w", err)
	}
	// 无论后续是否出错，结束时都更新这一行。
	defer s.finishLog(log)

	var errs []string
	defer func() {
		if r := recover(); r != nil {
			errs = append(errs, fmt.Sprintf("同步 panic: %v", r))
		}
		errBytes, _ := json.Marshal(errs)
		log.ErrorsJSON = string(errBytes)
	}()

	// 1. 拉部门树
	depts, err := s.client.WalkDepartments(ctx, s.cfg.RootDeptID)
	if err != nil {
		errs = append(errs, "拉取部门树失败: "+err.Error())
		log.Status = "failed"
		return log, err
	}

	// 2. 部门 → VPN 组（拓扑序：WalkDepartments 已保证父先于子）
	deptGroupMap, gErrs := s.reconcileGroups(ctx, depts)
	errs = append(errs, gErrs...)

	// 3. 拉成员（根部门 + 各部门），按 open_id 去重
	userDeptMap, uErrs := s.collectUsers(ctx, depts)
	errs = append(errs, uErrs...)

	// 4. 逐个 reconcile
	activeOpenIDs := make(map[string]struct{}, len(userDeptMap))
	for openID, fu := range userDeptMap {
		fu := fu
		activeOpenIDs[openID] = struct{}{}
		action, err := s.reconcileUser(ctx, fu, deptGroupMap)
		if err != nil {
			errs = append(errs, fmt.Sprintf("同步用户 %s(%s) 失败: %v", fu.Name, fu.OpenID, err))
			continue
		}
		switch action {
		case "created":
			log.Created++
		case "updated":
			log.Updated++
		}
	}
	log.TotalEmployees = len(userDeptMap)

	// 5. 离职检测
	if s.cfg.DisableOnLeave {
		n, dErrs := s.detectLeavers(ctx, activeOpenIDs)
		errs = append(errs, dErrs...)
		log.Disabled = n
	}

	if len(errs) > 0 && log.Created == 0 && log.Updated == 0 {
		log.Status = "failed"
	} else {
		log.Status = "success"
	}
	return log, nil
}

// finishLog 收尾：写 FinishedAt 与状态。已由 defer 调用。
func (s *FeishuSyncer) finishLog(log *FeishuSyncLog) {
	now := time.Now()
	log.FinishedAt = &now
	if log.Status == "" {
		log.Status = "failed"
	}
	_ = db.Model(&FeishuSyncLog{}).Where("id = ?", log.ID).Updates(map[string]interface{}{
		"status":          log.Status,
		"finished_at":     log.FinishedAt,
		"total_employees": log.TotalEmployees,
		"created":         log.Created,
		"updated":         log.Updated,
		"disabled":        log.Disabled,
		"errors":          log.ErrorsJSON,
	})
}

// reconcileGroups 把飞书部门映射为 VPN Group。返回 openDeptID → groupID 映射。
// 单次遍历即可（WalkDepartments 已按 DFS 前序返回，父先于子）。
func (s *FeishuSyncer) reconcileGroups(ctx context.Context, depts []FeishuDept) (map[string]uint, []string) {
	deptGroupMap := make(map[string]uint, len(depts))
	var errs []string

	// 取默认组 ID（根部门成员的兜底父节点）
	defaultGid := s.cfg.DefaultGroupID
	if defaultGid == 0 {
		var def Group
		if err := db.Where("name = ?", "Default").First(&def).Error; err == nil {
			defaultGid = def.ID
		}
	}

	for _, d := range depts {
		if d.OpenDeptID == "" || d.OpenDeptID == "0" {
			continue
		}
		var g Group
		result := db.Where("feishu_dept_id = ?", d.OpenDeptID).First(&g)

		// 计算父组：父为根 "0"（或空）→ 默认组；否则查已映射的父组
		parentGid := defaultGid
		if d.ParentOpenID != "" && d.ParentOpenID != "0" {
			if pid, ok := deptGroupMap[d.ParentOpenID]; ok {
				parentGid = pid
			}
		}

		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// 新建：FeishuDeptID 不走 BeforeSave，直接结构体赋值
			pg := parentGid
			newG := Group{
				Name:           d.Name,
				ParentID:       &pg,
				FeishuDeptID:   d.OpenDeptID,
				FeishuParentID: d.ParentOpenID,
			}
			if err := newG.Create(); err != nil {
				errs = append(errs, fmt.Sprintf("创建组 %s(%s) 失败: %v", d.Name, d.OpenDeptID, err))
				continue
			}
			deptGroupMap[d.OpenDeptID] = newG.ID
		} else if result.Error == nil {
			// 已存在：飞书只拥有 Name，其余不动
			deptGroupMap[d.OpenDeptID] = g.ID
			if g.Name != d.Name {
				if err := db.Model(&Group{}).Where("id = ?", g.ID).Update("name", d.Name).Error; err != nil {
					errs = append(errs, fmt.Sprintf("更新组 %s 名称失败: %v", d.OpenDeptID, err))
				}
			}
		} else {
			errs = append(errs, fmt.Sprintf("查询组 %s 失败: %v", d.OpenDeptID, result.Error))
		}
	}
	return deptGroupMap, errs
}

// collectUsers 拉取根部门及所有子部门的成员，按 open_id 去重，并记录每个用户的
// 主部门（取第一个非根部门；都为根则记 "0"）。返回 openID → FeishuUser。
func (s *FeishuSyncer) collectUsers(ctx context.Context, depts []FeishuDept) (map[string]FeishuUser, []string) {
	result := make(map[string]FeishuUser)
	var errs []string

	// 根部门成员
	rootUsers, err := s.client.ListUsersInDept(ctx, s.cfg.RootDeptID)
	if err != nil {
		errs = append(errs, "拉取根部门成员失败: "+err.Error())
	}
	mergeUsers(result, rootUsers, "0")

	// 各部门成员
	for _, d := range depts {
		if d.OpenDeptID == "" || d.OpenDeptID == "0" {
			continue
		}
		users, err := s.client.ListUsersInDept(ctx, d.OpenDeptID)
		if err != nil {
			errs = append(errs, fmt.Sprintf("拉取部门 %s 成员失败: %v", d.OpenDeptID, err))
			continue
		}
		mergeUsers(result, users, d.OpenDeptID)
	}
	return result, errs
}

// mergeUsers 把一批飞书用户合并进 result（已存在的 open_id 不覆盖，保留首次出现的主部门）。
func mergeUsers(result map[string]FeishuUser, users []FeishuUser, _ string) {
	for _, u := range users {
		if u.OpenID == "" {
			continue
		}
		if _, exists := result[u.OpenID]; !exists {
			result[u.OpenID] = u
		}
	}
}

// reconcileUser 对单个飞书用户做创建或更新。返回动作 "created" / "updated" / ""。
func (s *FeishuSyncer) reconcileUser(ctx context.Context, fu FeishuUser, deptGroupMap map[string]uint) (string, error) {
	now := time.Now()

	// 解析用户归属组：取第一个非根部门
	gid := s.cfg.DefaultGroupID
	for _, did := range fu.DepartmentIDs {
		if did != "" && did != "0" {
			if mapped, ok := deptGroupMap[did]; ok {
				gid = mapped
			}
			break
		}
	}

	var existing User
	findErr := db.Where("feishu_user_id = ?", fu.OpenID).First(&existing).Error

	if errors.Is(findErr, gorm.ErrRecordNotFound) {
		// 新增
		username := deriveUsername(fu)
		if username == "" {
			return "", errors.New("无法生成用户名（缺少 email/mobile/open_id）")
		}
		// 用户名与系统账号冲突或已存在则跳过
		if username == adminUsername {
			return "", errors.New("用户名与系统账户冲突")
		}
		var conflict User
		if db.Where("username = ?", username).First(&conflict).Error == nil {
			// 已有同名非飞书账号：不覆盖，仅记录 open_id 关联以便后续不重复
			return "", fmt.Errorf("用户名 %s 已被占用，跳过", username)
		}

		enable := true
		firstLogin := true
		plainPwd := generateDefaultPassword(fu.Mobile, fu.OpenID)
		u := User{
			Username:     username,
			Password:     plainPwd,
			IsEnable:     &enable,
			Name:         fu.Name,
			Email:        fu.Email,
			Phone:        normalizePhone(fu.Mobile),
			Gid:          gid,
			IsFirstLogin: &firstLogin,
			FeishuUserID: fu.OpenID,
			LastSyncAt:   &now,
		}
		if err := u.Create(); err != nil {
			return "", fmt.Errorf("创建用户失败: %w", err)
		}

		// 签发证书
		if err := ensureClientCert(username); err != nil {
			return "created", fmt.Errorf("签发证书失败: %w", err)
		}

		// 发欢迎邮件（best-effort，失败只记日志）
		if s.cfg.NotifyOnCreate && fu.Email != "" {
			if err := s.sendWelcomeEmail(fu.Name, username, plainPwd, fu.Email); err != nil {
				logger.Error(ctx, "发送欢迎邮件失败: "+err.Error())
			}
		}
		return "created", nil
	}

	if findErr != nil {
		return "", fmt.Errorf("查询用户失败: %w", findErr)
	}

	// 已存在：判断是否为复职（之前被离职禁用，现在又出现在飞书）
	isRejoin := existing.FeishuUserID == fu.OpenID && existing.IsEnable != nil && !*existing.IsEnable

	// 飞书拥有的字段用显式 allowlist 更新，绝不触碰管理员字段
	updates := map[string]interface{}{
		"name":           fu.Name,
		"email":          fu.Email,
		"phone":          normalizePhone(fu.Mobile),
		"gid":            gid,
		"is_enable":      true,
		"last_sync_at":   now,
		"feishu_user_id": fu.OpenID,
	}

	if isRejoin {
		// 复职：重新生成密码、强制改密、补发邮件
		plainPwd := generateDefaultPassword(fu.Mobile, fu.OpenID)
		updates["password"] = plainPwd
		updates["is_first_login"] = true
		if err := db.Model(&User{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return "", fmt.Errorf("复职更新失败: %w", err)
		}
		// 确保证书在（可能历史上被删）
		_ = ensureClientCert(existing.Username)
		if s.cfg.NotifyOnCreate && fu.Email != "" {
			if err := s.sendWelcomeEmail(fu.Name, existing.Username, plainPwd, fu.Email); err != nil {
				logger.Error(ctx, "复职发送邮件失败: "+err.Error())
			}
		}
		return "updated", nil
	}

	if err := db.Model(&User{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
		return "", fmt.Errorf("更新用户失败: %w", err)
	}
	return "updated", nil
}

// detectLeavers 禁用本次未出现的飞书映射账号。只改 is_enable 字段。
func (s *FeishuSyncer) detectLeavers(ctx context.Context, activeOpenIDs map[string]struct{}) (int, []string) {
	if len(activeOpenIDs) == 0 {
		return 0, nil
	}

	var leavers []User
	// 取所有启用中、有 feishu_user_id 的账号
	db.Where("feishu_user_id IS NOT NULL AND feishu_user_id != '' AND is_enable = ?", true).Find(&leavers)

	var errs []string
	disabled := 0
	for _, u := range leavers {
		if _, ok := activeOpenIDs[u.FeishuUserID]; ok {
			continue
		}
		if err := db.Model(&User{}).Where("id = ?", u.ID).Update("is_enable", false).Error; err != nil {
			errs = append(errs, fmt.Sprintf("禁用用户 %s 失败: %v", u.Username, err))
			continue
		}
		disabled++
	}
	return disabled, errs
}

// ResendWelcome 给指定本地用户重新生成/复用密码、确保证书、补发欢迎邮件（含 .ovpn 附件）。
// 供 admin UI 的"发送邮件"按钮调用。
func (s *FeishuSyncer) ResendWelcome(ctx context.Context, userID uint) error {
	var u User
	if err := db.First(&u, userID).Error; err != nil {
		return fmt.Errorf("用户不存在: %w", err)
	}
	if u.Email == "" {
		return errors.New("该用户未配置邮箱，无法发送")
	}

	// 密码：若仍处于首次登录态，沿用库里已存的明文密码（创建时生成）。
	// 注意 Password 经 AfterFind 解密为明文，可直接读。
	plainPwd := u.Password
	if plainPwd == "" {
		// 兜底：极少见，重新生成
		plainPwd = generateDefaultPassword(u.Phone, u.FeishuUserID)
		firstLogin := true
		if err := db.Model(&User{}).Where("id = ?", u.ID).Updates(map[string]interface{}{
			"password":        plainPwd,
			"is_first_login":  firstLogin,
		}).Error; err != nil {
			return fmt.Errorf("重置密码失败: %w", err)
		}
	}

	if err := ensureClientCert(u.Username); err != nil {
		return fmt.Errorf("签发证书失败: %w", err)
	}
	return s.sendWelcomeEmail(u.Name, u.Username, plainPwd, u.Email)
}

// ─────────────────────────────────────────────────────────────────────────────
// 辅助：用户名 / 密码 / 证书 / 邮件
// ─────────────────────────────────────────────────────────────────────────────

// deriveUsername 按优先级 email → mobile 后 6 位 → open_id 末段 生成登录用户名。
func deriveUsername(fu FeishuUser) string {
	if fu.Email != "" {
		return fu.Email
	}
	digits := stripNonDigits(fu.Mobile)
	if len(digits) >= 6 {
		return digits[len(digits)-6:]
	}
	if fu.UserID != "" {
		return fu.UserID
	}
	if fu.OpenID != "" {
		// open_id 形如 ou_xxxx，取末段字母数字
		s := stripToAlnum(fu.OpenID)
		if len(s) > 12 {
			return s[len(s)-12:]
		}
		return s
	}
	return ""
}

// generateDefaultPassword 生成默认密码：mobile 后 6 位（仅数字）+ 4 位随机大小写字母。
// mobile 不足 6 位时回退到 open_id 末段。
func generateDefaultPassword(mobile, openID string) string {
	var base string
	digits := stripNonDigits(mobile)
	if len(digits) >= 6 {
		base = digits[len(digits)-6:]
	} else {
		id := stripToAlnum(openID)
		if len(id) >= 6 {
			base = id[len(id)-6:]
		} else if id != "" {
			base = id
		} else {
			base = "000000"
		}
	}
	return base + randomMixedLetters(4)
}

const mixedLetters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// randomMixedLetters 用 crypto/rand 生成 n 位大小写混合字母，密码学安全。
func randomMixedLetters(n int) string {
	b := make([]byte, n)
	max := big.NewInt(int64(len(mixedLetters)))
	for i := range b {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			// 极端情况：退化为可预测值（不应发生）
			b[i] = mixedLetters[0]
			continue
		}
		b[i] = mixedLetters[idx.Int64()]
	}
	return string(b)
}

func stripNonDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func stripToAlnum(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// normalizePhone 规整手机号：去掉所有空白，保留 + 与数字，方便阅读与后续取后 6 位。
func normalizePhone(mobile string) string {
	mobile = strings.TrimSpace(mobile)
	var b strings.Builder
	for _, r := range mobile {
		if r == '+' || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ensureClientCert 确保 clients/<username>.ovpn 存在，不存在则调用 genclient 签发。
// 与 POST /ovpn/client 的 exec.Command 模式一致。
func ensureClientCert(username string) error {
	clientsDir := filepath.Join(ovData, "clients")
	if err := os.MkdirAll(clientsDir, 0755); err != nil {
		return err
	}
	ovpnPath := filepath.Join(clientsDir, username+".ovpn")
	if _, err := os.Stat(ovpnPath); err == nil {
		return nil // 已存在
	}

	// 参数顺序：name serverAddr serverPort config ccdConfig mfa
	// 空的 serverAddr/serverPort → genclient 用 ip route 自动探测公网地址
	cmd := exec.Command("docker-entrypoint.sh", "genclient", username, "", "", "", "", "false")
	if out, err := cmd.CombinedOutput(); err != nil {
		if len(out) == 0 {
			out = []byte(err.Error())
		}
		return fmt.Errorf("genclient 失败: %s", string(out))
	}
	return nil
}

// sendWelcomeEmail 渲染 email.html 并附上 .ovpn 附件发送。subject 为固定标题。
func (s *FeishuSyncer) sendWelcomeEmail(name, username, password, email string) error {
	var buf bytes.Buffer
	tpl, err := template.ParseFS(FS, "templates/email.html")
	if err != nil {
		return fmt.Errorf("解析邮件模板失败: %w", err)
	}
	if err := tpl.Execute(&buf, map[string]interface{}{
		"Type":         "addUser",
		"Name":         name,
		"Username":     username,
		"Password":     password,
		"SiteUrl":      viper.GetString("system.base.site_url"),
		"HasAttachment": true,
	}); err != nil {
		return fmt.Errorf("渲染邮件失败: %w", err)
	}

	var attachments []EmailAttachment
	if f, err := os.Open(filepath.Join(ovData, "clients", username+".ovpn")); err == nil {
		attachments = append(attachments, EmailAttachment{
			Filename: username + ".ovpn",
			Reader:   f,
		})
		// 注意：sendEmail 在 DialAndSend 时读 reader，调用结束后再关文件。
		defer f.Close()
	}

	return sendEmail(email, "VPN账号开通通知", buf.String(), attachments...)
}

// FeishuSyncLogRecent 返回最近 n 条同步日志（按时间倒序）。
func FeishuSyncLogRecent(n int) []FeishuSyncLog {
	var logs []FeishuSyncLog
	db.Order("id desc").Limit(n).Find(&logs)
	return logs
}
