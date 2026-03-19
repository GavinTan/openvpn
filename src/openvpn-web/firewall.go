package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Firewall struct {
	ID        uint      `gorm:"primarykey" json:"id" form:"id"`
	SIP       string    `gorm:"column:sip" json:"sip" form:"sip"`
	DIP       string    `gorm:"column:dip" json:"dip" form:"dip"`
	SGroup    []*Group  `gorm:"many2many:firewall_sgroup;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"sg"`
	DGroup    []*Group  `gorm:"many2many:firewall_dgroup;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"dg"`
	Policy    string    `json:"policy" form:"policy"`
	Status    *bool     `gorm:"default:true" form:"status" json:"status"`
	Comment   string    `json:"comment" form:"comment"`
	CreatedAt time.Time `json:"createdAt,omitempty" form:"createdAt,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty" form:"updatedAt,omitempty"`
}

type ChainRuleData struct {
	Rate   string `json:"rate"`
	Unit   string `json:"unit"`
	Handle string `json:"handle"`
}

type FirewallSet struct {
	SetName string `json:"set_name"`
}

func getNftQosChainRule(chain, ip string) ChainRuleData {
	cmd := exec.Command(
		"nft",
		"-a",
		"list",
		"chain",
		"inet",
		nftTableName,
		chain,
	)

	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, ip) {
				re := regexp.MustCompile(`over (\d+)\s+(\S+).*handle (\d+)`)
				m := re.FindStringSubmatch(line)
				if len(m) == 4 {
					return ChainRuleData{
						Rate:   m[1],
						Unit:   m[2],
						Handle: m[3],
					}
				}
			}
		}
	}

	return ChainRuleData{}
}

func setNftQosChain(chain, ips, rate, unit string) error {
	r, _ := strconv.Atoi(rate)
	if r < 0 {
		return nil
	}

	addrField := "daddr"
	if chain == "upload" {
		addrField = "saddr"
	}

	for ip := range strings.SplitSeq(ips, ",") {
		qos := getNftQosChainRule(chain, ip)
		if qos.Handle == "" && r == 0 {
			return nil
		}

		family := "ip"
		if strings.Contains(ip, ":") {
			family = "ip6"
		}

		cmd := exec.Command("nft", "add", "rule", "inet", nftTableName, chain, family, addrField, ip, "limit", "rate", "over", rate, unit, "drop")
		if qos.Handle != "" {
			if r == 0 {
				cmd = exec.Command("nft", "delete", "rule", "inet", nftTableName, chain, "handle", qos.Handle)
			} else {
				cmd = exec.Command("nft", "replace", "rule", "inet", nftTableName, chain, "handle", qos.Handle, family, addrField, ip, "limit", "rate", "over", rate, unit, "drop")
			}
		}

		if out, err := cmd.CombinedOutput(); err != nil {
			if len(out) == 0 {
				out = []byte(err.Error())
			}

			return fmt.Errorf("%s", out)
		}
	}

	return nil
}

func setNftBlackList(ips, action string) error {
	var sb strings.Builder
	var v4, v6 []string

	for ip := range strings.SplitSeq(ips, ",") {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}

		if strings.Contains(ip, ":") {
			v6 = append(v6, ip)
		} else {
			v4 = append(v4, ip)
		}
	}

	if len(v4) > 0 {
		fmt.Fprintf(&sb, "%s element inet %s blacklist_v4 { %s }\n", action, nftTableName, strings.Join(v4, ", "))
	}

	if len(v6) > 0 {
		fmt.Fprintf(&sb, "%s element inet %s blacklist_v6 { %s }\n", action, nftTableName, strings.Join(v6, ", "))
	}

	if sb.Len() == 0 {
		return nil
	}

	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(sb.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		if len(out) == 0 {
			out = []byte(err.Error())
		}

		return fmt.Errorf("%s", out)
	}

	return nil
}

func getNftChainRule(chain, comment string) ChainRuleData {
	cmd := exec.Command(
		"nft",
		"-a",
		"list",
		"chain",
		"inet",
		nftTableName,
		chain,
	)

	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, fmt.Sprintf("comment \"%s\"", comment)) {
				return ChainRuleData{
					Handle: strings.TrimSpace(line[strings.Index(line, "handle")+len("handle"):]),
				}
			}
		}
	}

	return ChainRuleData{}
}

func setNftChain(chain string, f Firewall) error {
	var sb strings.Builder
	sSetName := fmt.Sprintf("f%d_src", f.ID)
	dSetName := fmt.Sprintf("f%d_dst", f.ID)

	if err := setNftTableSet(sSetName, f.SIP); err != nil {
		return err
	}

	if err := setNftTableSet(dSetName, f.DIP); err != nil {
		return err
	}

	for _, suffix := range []string{"v4", "v6"} {
		sSetName := fmt.Sprintf("f%d_src_%s", f.ID, suffix)
		dSetName := fmt.Sprintf("f%d_dst_%s", f.ID, suffix)

		if f.SIP == "" {
			fmt.Fprintf(&sb, "flush set inet %s %s\n", nftTableName, sSetName)
		}

		if f.DIP == "" {
			fmt.Fprintf(&sb, "flush set inet %s %s\n", nftTableName, dSetName)
		}

		family := "ip"
		if suffix == "v6" {
			family = "ip6"
		}

		policy := f.Policy
		if f.Status != nil && !*f.Status {
			policy = "continue"
		}

		rule := getNftChainRule(chain, fmt.Sprintf("id_%d_%s", f.ID, suffix))
		if rule.Handle != "" {
			fmt.Fprintf(&sb, "replace rule inet %s %s handle %s %s saddr @%s %s daddr @%s %s comment \"id_%d_%s\"\n", nftTableName, chain, rule.Handle, family, sSetName, family, dSetName, policy, f.ID, suffix)
		} else {
			fmt.Fprintf(&sb, "add rule inet %s %s %s saddr @%s %s daddr @%s %s comment \"id_%d_%s\"\n", nftTableName, chain, family, sSetName, family, dSetName, policy, f.ID, suffix)
		}
	}

	if sb.Len() == 0 {
		return nil
	}

	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(sb.String())

	if out, err := cmd.CombinedOutput(); err != nil {
		if len(out) == 0 {
			out = []byte(err.Error())
		}

		return fmt.Errorf("%s", out)
	}

	return nil
}

func deleteNftChainRule(chain, fid string) error {
	var sb strings.Builder
	for _, suffix := range []string{"v4", "v6"} {
		rule := getNftChainRule(chain, fmt.Sprintf("id_%s_%s", fid, suffix))
		if rule.Handle != "" {
			fmt.Fprintf(&sb, "delete rule inet %s %s handle %s\n", nftTableName, chain, rule.Handle)
		}

		setName := fmt.Sprintf("f%s_src_%s", fid, suffix)
		if getNftTableSet(setName) {
			fmt.Fprintf(&sb, "delete set inet %s %s\n", nftTableName, setName)
		}

		setName = fmt.Sprintf("f%s_dst_%s", fid, suffix)
		if getNftTableSet(setName) {
			fmt.Fprintf(&sb, "delete set inet %s %s\n", nftTableName, setName)
		}
	}

	if sb.Len() == 0 {
		return nil
	}

	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(sb.String())

	if out, err := cmd.CombinedOutput(); err != nil {
		if len(out) == 0 {
			out = []byte(err.Error())
		}

		return fmt.Errorf("%s", out)
	}

	return nil
}

func setNftTableSet(name, ips string) error {
	var v4, v6 []string
	v4SetName := name + "_v4"
	v6SetName := name + "_v6"

	for ip := range strings.SplitSeq(ips, ",") {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}

		if strings.Contains(ip, ":") {
			v6 = append(v6, ip)
		} else {
			v4 = append(v4, ip)
		}
	}

	var sb strings.Builder

	fmt.Fprintf(&sb, "add set inet %s %s { type ipv4_addr; flags interval; }\n", nftTableName, v4SetName)
	if len(v4) > 0 {
		fmt.Fprintf(&sb, "flush set inet %s %s\n", nftTableName, v4SetName)
		fmt.Fprintf(&sb, "add element inet %s %s { %s }\n", nftTableName, v4SetName, strings.Join(v4, ", "))
	}

	fmt.Fprintf(&sb, "add set inet %s %s { type ipv6_addr; flags interval; }\n", nftTableName, v6SetName)
	if len(v6) > 0 {
		fmt.Fprintf(&sb, "flush set inet %s %s\n", nftTableName, v6SetName)
		fmt.Fprintf(&sb, "add element inet %s %s { %s }\n", nftTableName, v6SetName, strings.Join(v6, ", "))
	}

	if sb.Len() == 0 {
		return nil
	}

	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(sb.String())

	if out, err := cmd.CombinedOutput(); err != nil {
		if len(out) == 0 {
			out = []byte(err.Error())
		}

		return fmt.Errorf("%s", out)
	}

	return nil
}

func deleteNftTableSet(name string) error {
	if getNftTableSet(name) {
		cmd := exec.Command("nft", "delete", "set", "inet", nftTableName, name)
		if out, err := cmd.CombinedOutput(); err != nil {
			if len(out) == 0 {
				out = []byte(err.Error())
			}

			return fmt.Errorf("%s", out)
		}
	}

	return nil
}

func getNftTableSet(name string) bool {
	return exec.Command("nft", "list", "set", "inet", nftTableName, name).Run() == nil
}

func getNftTableSetElement(name, ip string) bool {
	setName := name + "_v4"
	if strings.Contains(ip, ":") {
		setName = name + "_v6"
	}

	cmd := exec.Command(
		"nft", "get", "element", "inet",
		nftTableName,
		setName,
		fmt.Sprintf("{ %s }", ip),
	)
	if out, err := cmd.Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, ip) {
				return true
			}
		}
	}

	return false
}

func addNftTableSetElement(name, ips string) error {
	var v4, v6 []string
	var sb strings.Builder

	v4SetName := name + "_v4"
	v6SetName := name + "_v6"

	for ip := range strings.SplitSeq(ips, ",") {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}

		if getNftTableSetElement(name, ip) {
			continue
		}

		if strings.Contains(ip, ":") {
			v6 = append(v6, ip)
		} else {
			v4 = append(v4, ip)
		}
	}

	if len(v4) > 0 {
		fmt.Fprintf(&sb, "add element inet %s %s { %s }\n", nftTableName, v4SetName, strings.Join(v4, ", "))
	}

	if len(v6) > 0 {
		fmt.Fprintf(&sb, "add element inet %s %s { %s }\n", nftTableName, v6SetName, strings.Join(v6, ", "))
	}

	if sb.Len() == 0 {
		return nil
	}

	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(sb.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		if len(out) == 0 {
			out = []byte(err.Error())
		}

		return fmt.Errorf("%s", out)
	}

	return nil
}

func deleteNftTableSetElement(name, ips string) error {
	for ip := range strings.SplitSeq(ips, ",") {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}

		setName := name + "_v4"
		if strings.Contains(ip, ":") {
			setName = name + "_v6"
		}

		if getNftTableSetElement(name, ip) {
			cmd := exec.Command(
				"nft", "delete", "element", "inet",
				nftTableName,
				setName,
				fmt.Sprintf("{ %s }", ip),
			)

			if out, err := cmd.CombinedOutput(); err != nil {
				if len(out) == 0 {
					out = []byte(err.Error())
				}

				return fmt.Errorf("%s", out)
			}
		}
	}

	return nil
}

func setOnlineClinetNft(f Firewall) error {
	updateSetElement := func(exist bool, setName string, ips string) error {
		if exist {
			return addNftTableSetElement(setName, ips)
		}
		return deleteNftTableSetElement(setName, ips)
	}

	sGroupMap := make(map[string]struct{}, len(f.SGroup))
	for _, g := range f.SGroup {
		sGroupMap[strconv.Itoa(int(g.ID))] = struct{}{}
	}

	dGroupMap := make(map[string]struct{}, len(f.DGroup))
	for _, g := range f.DGroup {
		dGroupMap[strconv.Itoa(int(g.ID))] = struct{}{}
	}

	ov := ovpn{address: ovManage}
	onlineClients := ov.getClient()
	for _, client := range onlineClients {
		sexist := false
		dexist := false

		if client.Username == "UNDEF" {
			return nil
		}

		u := User{Username: client.Username}
		groups := u.GetGroups()
		for _, g := range groups {
			id := strconv.Itoa(int(g.ID))

			if !sexist {
				if _, ok := sGroupMap[id]; ok {
					sexist = true
				}
			}

			if !dexist {
				if _, ok := dGroupMap[id]; ok {
					dexist = true
				}
			}

			if sexist && dexist {
				break
			}
		}

		if err := updateSetElement(sexist, fmt.Sprintf("f%d_src", f.ID), fmt.Sprintf("%s,%s", client.Vip, client.Vip6)); err != nil {
			logger.Error(context.Background(), err.Error())
			return err
		}

		if err := updateSetElement(dexist, fmt.Sprintf("f%d_dst", f.ID), fmt.Sprintf("%s,%s", client.Vip, client.Vip6)); err != nil {
			logger.Error(context.Background(), err.Error())
			return err
		}

	}

	return nil
}

func saveNftConfig() error {
	cmd := exec.Command("nft", "list", "table", "inet", nftTableName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) == 0 {
			out = []byte(err.Error())
		}

		return fmt.Errorf("%s", out)
	}

	return os.WriteFile(path.Join(ovData, "openvpn.nft"), out, 0644)
}

func getUserFirewallSetName(username string) []FirewallSet {
	var fs []FirewallSet

	db.Raw(`
		WITH RECURSIVE group_tree AS (
			SELECT g.id, g.parent_id
			FROM "group" g
			JOIN user u ON u.gid = g.id
			WHERE u.username = ?

			UNION ALL

			SELECT g.id, g.parent_id
			FROM "group" g
			INNER JOIN group_tree gt ON g.id = gt.parent_id
		)
		SELECT 'f' || f.id || '_src' AS set_name
		FROM firewall f
		JOIN firewall_sgroup fs ON fs.firewall_id = f.id
		WHERE fs.group_id IN (SELECT id FROM group_tree)

		UNION ALL

		SELECT 'f' || f.id || '_dst' AS set_name
		FROM firewall f
		JOIN firewall_dgroup fd ON fd.firewall_id = f.id
		WHERE fd.group_id IN (SELECT id FROM group_tree)
	`, username).Scan(&fs)

	return fs
}

func (f *Firewall) Get(id string) error {
	result := db.First(&f, id)
	return result.Error
}

func (f *Firewall) All() []Firewall {
	var firewalls []Firewall

	result := db.Model(&Firewall{}).Preload("SGroup").Preload("DGroup").Find(&firewalls)
	if result.Error != nil {
		logger.Error(context.Background(), result.Error.Error())
		return []Firewall{}
	}

	return firewalls
}

func (f *Firewall) Create() error {
	result := db.Omit("SGroup.*", "DGroup.*").Create(&f)
	return result.Error
}

func (f *Firewall) Update() error {
	result := db.Model(&f).Omit("SGroup.*", "DGroup.*").Updates(&f)
	return result.Error
}

func (f *Firewall) Delete(id string) error {
	result := db.Delete(&f, id)
	return result.Error
}

func (f *Firewall) TableName() string {
	return "firewall"
}

func FirewallHandler(c *gin.Context) {
	switch c.Request.Method {
	case http.MethodGet:
		a := c.Query("a")
		switch a {
		case "get_rateLimit":
			vip := c.Query("vip")
			upQos := getNftQosChainRule("upload", vip)
			downQos := getNftQosChainRule("download", vip)

			c.JSON(http.StatusOK, gin.H{"upQos": upQos, "downQos": downQos})
		default:
			var f Firewall
			c.JSON(http.StatusOK, f.All())
		}
	case http.MethodPost:
		a := c.Query("a")
		switch a {
		case "add_blacklist":
			vip := c.PostForm("vip")

			err := setNftBlackList(vip, "add")
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": "禁网失败"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "禁网成功"})
		case "remove_blacklist":
			vip := c.PostForm("vip")

			err := setNftBlackList(vip, "delete")
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": "解除网络限制失败"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "解除网络限制成功"})
		case "set_rateLimit":
			vip := c.PostForm("vip")
			upload := c.PostForm("upload")
			uploadUnit := c.PostForm("uploadUnit")
			download := c.PostForm("download")
			downloadUnit := c.PostForm("downloadUnit")

			err := setNftQosChain("upload", vip, upload, uploadUnit)
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": "设置上传速率失败"})
				return
			}

			err = setNftQosChain("download", vip, download, downloadUnit)
			if err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": "设置下载速率失败"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "设置速率成功"})
		case "add_ovips":
			username := c.PostForm("username")
			vip := c.PostForm("vip")
			vip6 := c.PostForm("vip6")

			fs := getUserFirewallSetName(username)
			for _, f := range fs {
				err := addNftTableSetElement(f.SetName, fmt.Sprintf("%s,%s", vip, vip6))
				if err != nil {
					logger.Error(context.Background(), err.Error())
					c.JSON(http.StatusInternalServerError, gin.H{"message": "设置防火墙策略失败"})
					return
				}
			}

			if err := saveNftConfig(); err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": "保存防火墙配置失败"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "设置防火墙策略成功"})
		case "delete_ovips":
			username := c.PostForm("username")
			vip := c.PostForm("vip")
			vip6 := c.PostForm("vip6")

			fs := getUserFirewallSetName(username)
			for _, f := range fs {
				err := deleteNftTableSetElement(f.SetName, fmt.Sprintf("%s,%s", vip, vip6))
				if err != nil {
					logger.Error(context.Background(), err.Error())
					c.JSON(http.StatusInternalServerError, gin.H{"message": "移除防火墙策略失败"})
					return
				}
			}

			if err := saveNftConfig(); err != nil {
				logger.Error(context.Background(), err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"message": "保存防火墙配置失败"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "移除防火墙策略成功"})
		default:
			var f Firewall
			c.ShouldBind(&f)

			sg := c.PostForm("sg")
			if sg != "" {
				for _, g := range strings.Split(sg, ",") {
					id, _ := strconv.Atoi(g)
					f.SGroup = append(f.SGroup, &Group{ID: uint(id)})
				}
			}
			dg := c.PostForm("dg")
			if dg != "" {
				for _, g := range strings.Split(dg, ",") {
					id, _ := strconv.Atoi(g)
					f.DGroup = append(f.DGroup, &Group{ID: uint(id)})
				}
			}

			err := db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Omit("SGroup.*", "DGroup.*").Create(&f).Error; err != nil {
					return err
				}

				err := setNftChain("forward", f)
				if err != nil {
					logger.Error(context.Background(), err.Error())
					return fmt.Errorf("创建防火墙规则失败")
				}

				if err := setOnlineClinetNft(f); err != nil {
					logger.Error(context.Background(), err.Error())
					return fmt.Errorf("添加在线客户端防火墙规则失败")
				}

				if err := saveNftConfig(); err != nil {
					logger.Error(context.Background(), err.Error())
					return fmt.Errorf("保存防火墙配置失败")
				}

				return nil
			})

			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			} else {
				c.JSON(http.StatusOK, gin.H{"message": "添加成功"})
			}
		}
	case http.MethodPatch:
		var f Firewall
		c.ShouldBind(&f)

		err := db.Transaction(func(tx *gorm.DB) error {
			if sg, ok := c.Request.PostForm["sg"]; ok {
				if sg[0] != "" {
					for _, g := range strings.Split(sg[0], ",") {
						id, _ := strconv.Atoi(g)
						f.SGroup = append(f.SGroup, &Group{ID: uint(id)})
					}
				}

				if err := tx.Model(&f).Omit("SGroup.*", "DGroup.*").Association("SGroup").Replace(f.SGroup); err != nil {
					return err
				}
			}

			if dg, ok := c.Request.PostForm["dg"]; ok {
				if dg[0] != "" {
					for _, g := range strings.Split(dg[0], ",") {
						id, _ := strconv.Atoi(g)
						f.DGroup = append(f.DGroup, &Group{ID: uint(id)})
					}
				}

				if err := tx.Model(&f).Omit("SGroup.*", "DGroup.*").Association("DGroup").Replace(f.DGroup); err != nil {
					return err
				}
			}

			if err := setNftChain("forward", f); err != nil {
				logger.Error(context.Background(), err.Error())
				return fmt.Errorf("更新防火墙失败")
			}

			if err := setOnlineClinetNft(f); err != nil {
				logger.Error(context.Background(), err.Error())
				return fmt.Errorf("更新在线客户端防火墙规则失败")
			}

			if err := saveNftConfig(); err != nil {
				logger.Error(context.Background(), err.Error())
				return fmt.Errorf("保存防火墙配置失败")
			}

			if err := tx.Omit("SGroup.*", "DGroup.*").Updates(&f).Error; err != nil {
				return err
			}

			if sip, ok := c.Request.PostForm["sip"]; ok {
				if sip[0] == "" {
					if err := tx.Model(&f).Omit("SGroup.*", "DGroup.*").Update("sip", "").Error; err != nil {
						return err
					}
				}
			}

			if dip, ok := c.Request.PostForm["dip"]; ok {
				if dip[0] == "" {
					if err := tx.Model(&f).Omit("SGroup.*", "DGroup.*").Update("dip", "").Error; err != nil {
						return err
					}
				}
			}

			return nil
		})

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		} else {
			c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
		}
	case http.MethodDelete:
		var f Firewall
		id := c.Param("id")

		err := db.Transaction(func(tx *gorm.DB) error {
			if err := deleteNftChainRule("forward", id); err != nil {
				logger.Error(context.Background(), err.Error())
				return fmt.Errorf("删除防火墙失败")
			}

			if err := saveNftConfig(); err != nil {
				logger.Error(context.Background(), err.Error())
				return fmt.Errorf("保存防火墙配置失败")
			}

			if err := tx.Delete(&f, id).Error; err != nil {
				return err
			}

			return nil
		})

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		} else {
			c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
		}
	}
}
