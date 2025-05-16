package auth

import (
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// IPWhitelistMiddleware 创建一个 IP 白名单中间件
func IPWhitelistMiddleware(whitelist []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()

		// 检查客户端 IP 是否在白名单中
		allowed := false
		for _, ip := range whitelist {
			if ip == clientIP {
				allowed = true
				break
			}
		}

		// 如果 IP 不在白名单中，返回 403 Forbidden
		if !allowed {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"message": "Access denied",
			})
			zap.L().Info("bloack ip", zap.String("ip", clientIP), zap.String("resource", c.Request.URL.Path))
			return
		}

		// IP 在白名单中，继续处理请求
		c.Next()
	}
}

// IPRangeWhitelistMiddleware 创建一个支持 IP 段的 IP 白名单中间件
func IPRangeWhitelistMiddleware(whitelist []string) gin.HandlerFunc {
	// 解析白名单中的 CIDR
	var allowedNetworks []*net.IPNet
	for _, cidr := range whitelist {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			panic("Invalid CIDR in whitelist: " + cidr)
		}
		allowedNetworks = append(allowedNetworks, network)
	}

	return func(c *gin.Context) {
		clientIP := net.ParseIP(c.ClientIP())

		// 检查客户端 IP 是否在任何一个允许的 CIDR 中
		allowed := false
		for _, network := range allowedNetworks {
			if network.Contains(clientIP) {
				allowed = true
				break
			}
		}

		// 如果 IP 不在白名单中，返回 403 Forbidden
		if !allowed {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"message": "Access denied",
			})
			return
		}

		// IP 在白名单中，继续处理请求
		c.Next()
	}
}

func IntranetOnly() gin.HandlerFunc {
	return IPRangeWhitelistMiddleware([]string{"127.0.0.1/32", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
}
