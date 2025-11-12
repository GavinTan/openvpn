package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path"
	"time"

	"github.com/go-ldap/ldap/v3"
)

type LdapUserData struct {
	Username   string `json:"username"`
	LdapAuth   bool   `json:"ldapAuth"`
	Ipaddr     string `json:"ipaddr"`
	OvpnConfig string `json:"ovpnConfig"`
}

type LdapConn struct {
	Conn *ldap.Conn
}

func InitLdap() (*LdapConn, error) {
	l, err := ldap.DialURL(ldapURL, ldap.DialWithTLSConfig(&tls.Config{InsecureSkipVerify: true}), ldap.DialWithDialer(&net.Dialer{
		Timeout: 5 * time.Second,
	}))
	if err != nil {
		return nil, fmt.Errorf("ldap connect error: %v", err)
	}

	err = l.Bind(ldapBindUserDn, ldapBindPassword)
	if err != nil {
		return nil, fmt.Errorf("ldap bind error: %v", err)
	}

	return &LdapConn{Conn: l}, nil
}

func (l *LdapConn) LdapSearch(username string) (*ldap.SearchResult, error) {
	f := fmt.Sprintf("(%s=%s)", ldapUserAttribute, ldap.EscapeFilter(username))
	if ldapUserGroupFilter {
		f = fmt.Sprintf("(&(%s=%s)(memberOf=%s))", ldapUserAttribute, ldap.EscapeFilter(username), ldap.EscapeFilter(ldapUserGroupDn))
	}

	searchRequest := ldap.NewSearchRequest(
		ldapBaseDn,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		f,
		[]string{"dn", ldapUserAttrIpaddrName, ldapUserAttrConfigName},
		nil,
	)

	sr, err := l.Conn.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	if len(sr.Entries) == 0 {
		return nil, fmt.Errorf("用户不存在")
	}

	if len(sr.Entries) > 1 {
		return nil, fmt.Errorf("用户存在多个条目")
	}

	return sr, nil
}

func (l *LdapConn) Auth(username, password string) error {
	sr, err := l.LdapSearch(username)
	if err != nil {
		return err
	}

	userdn := sr.Entries[0].DN

	err = l.Conn.Bind(userdn, password)
	if err != nil {
		return err
	}

	defer l.Conn.Close()

	ipaddr := sr.Entries[0].GetAttributeValue(ldapUserAttrIpaddrName)
	if ipaddr != "" {
		os.WriteFile(path.Join(ovData, ".ovip"), []byte(ipaddr), 0644)
	}

	return nil
}

func (l *LdapConn) Get(username string) (LdapUserData, error) {
	sr, err := l.LdapSearch(username)
	if err != nil {
		return LdapUserData{}, err
	}

	return LdapUserData{
		Username:   username,
		LdapAuth:   ldapAuth,
		Ipaddr:     sr.Entries[0].GetAttributeValue(ldapUserAttrIpaddrName),
		OvpnConfig: sr.Entries[0].GetAttributeValue(ldapUserAttrConfigName),
	}, nil
}
