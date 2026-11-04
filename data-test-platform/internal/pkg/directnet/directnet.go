// Package directnet 判断目标主机是否应绕过系统代理直连。
//
// 内部域名、私网地址、以及 .local 后缀走直连，避免被 Clash / Surge 之类的
// 系统代理拦截或改写。额外后缀由调用方提供，代码里不绑定任何具体企业域名。
package directnet

import (
	"net"
	"strings"
)

// matcher 保存额外的直连后缀，一般来自项目配置。
type matcher struct {
	suffixes []string
}

var defaultMatcher = matcher{}

// SetExtraSuffixes 注册额外的直连域名后缀，如 ".example.com"。
// 传入的项会自动补上前导点并转小写。
func SetExtraSuffixes(suffixes []string) {
	normalized := make([]string, 0, len(suffixes))
	for _, s := range suffixes {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if !strings.HasPrefix(s, ".") {
			s = "." + s
		}
		normalized = append(normalized, s)
	}
	defaultMatcher.suffixes = normalized
}

// IsDirect 判断该 host 是否应绕过系统代理。
func IsDirect(host string) bool {
	return defaultMatcher.IsDirect(host)
}

// IsDirect 判断该 host 是否应绕过系统代理。
func (m matcher) IsDirect(host string) bool {
	if host == "" {
		return false
	}
	host = strings.ToLower(host)

	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	if strings.HasSuffix(host, ".local") {
		return true
	}

	// 私网地址段。net.ParseIP 判断比字符串前缀可靠，能覆盖 172.16/12 全段。
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return true
		}
		return false
	}

	for _, s := range m.suffixes {
		if strings.HasSuffix(host, s) {
			return true
		}
	}
	return false
}
