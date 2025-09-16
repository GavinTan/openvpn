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

var (
	ldapURL  = os.Getenv("LDAP_URL")
	baseDN   = os.Getenv("LDAP_BASE_DN")
	userAttr = os.Getenv("LDAP_USER_ATTRIBUTE")

	ugFilter = os.Getenv("LDAP_USER_GROUP_FILTER")
	ugDN     = os.Getenv("LDAP_USER_GROUP_DN")
	uIpaddr  = os.Getenv("LDAP_USER_ATTR_IPADDR_NAME")

	bu = os.Getenv("LDAP_BIND_USER_DN")
	bp = os.Getenv("LDAP_BIND_PASSWORD")
)

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

	err = l.Bind(bu, bp)
	if err != nil {
		return nil, fmt.Errorf("ldap bind error: %v", err)
	}

	return &LdapConn{Conn: l}, nil
}

func (l *LdapConn) LdapSearch(username string) (*ldap.SearchResult, error) {
	f := fmt.Sprintf("(%s=%s)", userAttr, ldap.EscapeFilter(username))
	if ugFilter == "true" {
		f = fmt.Sprintf("(&(%s=%s)(memberOf=%s))", userAttr, ldap.EscapeFilter(username), ldap.EscapeFilter(ugDN))
	}

	searchRequest := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		f,
		[]string{"dn", uIpaddr},
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

	ipaddr := sr.Entries[0].GetAttributeValue(uIpaddr)
	if ipaddr != "" {
		ovData, ok := os.LookupEnv("OVPN_DATA")
		if !ok {
			ovData = "/data"
		}

		os.WriteFile(path.Join(ovData, ".ovip"), []byte(ipaddr), 0644)
	}

	return nil
}
