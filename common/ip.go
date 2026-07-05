package common

import (
	"net"
	"net/netip"
	"strings"

	"github.com/gin-gonic/gin"
)

const InternalRealIPHeader = "X-New-Api-Real-IP"

func IsInternalRequestHeader(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), InternalRealIPHeader)
}

func GetClientIP(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if value := strings.TrimSpace(c.GetHeader(InternalRealIPHeader)); value != "" {
		if _, err := netip.ParseAddr(value); err == nil {
			return value
		}
	}
	return c.ClientIP()
}

func IsIpInCIDRList(ip net.IP, list []string) bool {
	if ip == nil {
		return false
	}
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			_, network, err := net.ParseCIDR(item)
			if err == nil && network != nil && network.Contains(ip) {
				return true
			}
			continue
		}
		parsedIP := net.ParseIP(item)
		if parsedIP != nil && parsedIP.Equal(ip) {
			return true
		}
	}
	return false
}
