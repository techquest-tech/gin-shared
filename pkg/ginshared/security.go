//go:build securityResp || all

package ginshared

import "github.com/gin-gonic/gin"

func init() {
	RegisterComponent(&SecurityResp{})
}

type SecurityResp struct {
	DefaultComponent
}

func (*SecurityResp) OnEngineInited(r *gin.Engine) error {
	r.Use(func(c *gin.Context) {
		// for CSRF
		c.Header("Set-Cookie", "dummy-cookie=none; Path=/; Domain="+c.Request.Host+"; Max-Age=0; HttpOnly; Secure; SameSite=Lax")
		// for X-Frame-Options
		c.Header("X-Frame-Options", "DENY") // 兼容老浏览器
		// for Content-Security-Policy
		c.Header("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com https://cdn.jsdelivr.net; "+
				"style-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com https://cdn.jsdelivr.net; "+
				"img-src 'self' data: https:; "+
				"font-src 'self' https://cdnjs.cloudflare.com https://cdn.jsdelivr.net; "+
				"frame-ancestors 'none'; "+
				"base-uri 'self'; "+
				"form-action 'self'; "+
				"upgrade-insecure-requests; "+
				"block-all-mixed-content")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")

		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=(), usb=(), vr=(), accelerometer=(), gyroscope=(), magnetometer=(), fullscreen=(self), interest-cohort=()")
		c.Next()
	})
	return nil
}
