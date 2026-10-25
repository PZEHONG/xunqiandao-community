package web

import (
	"net"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

type transportPolicy struct {
	proxies       []string
	networks      []*net.IPNet
	secureCookies bool
}

// Only explicitly configured proxy peers may supply forwarded identity or TLS.
// The default matches the loopback Nginx deployment and loopback-only listener.
func loadTransportPolicy() transportPolicy {
	raw, configured := os.LookupEnv("XIXUN_TRUSTED_PROXIES")
	if !configured {
		raw = "127.0.0.1/32,::1/128"
	}
	policy := transportPolicy{}
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if ip := net.ParseIP(value); ip != nil {
			bits := 128
			if ip.To4() != nil {
				ip = ip.To4()
				bits = 32
			}
			policy.networks = append(policy.networks, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
		} else {
			_, network, err := net.ParseCIDR(value)
			if err != nil {
				panic("invalid XIXUN_TRUSTED_PROXIES: expected IP addresses or CIDRs")
			}
			ones, _ := network.Mask.Size()
			if ones == 0 {
				panic("XIXUN_TRUSTED_PROXIES must not trust all internet clients")
			}
			policy.networks = append(policy.networks, network)
		}
		policy.proxies = append(policy.proxies, value)
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("XIXUN_SECURE_COOKIES"))) {
	case "", "auto":
	case "true", "1":
		policy.secureCookies = true
	default:
		panic("XIXUN_SECURE_COOKIES must be auto or true")
	}
	return policy
}

func (p transportPolicy) trustsPeer(requestPeer string) bool {
	host, _, err := net.SplitHostPort(requestPeer)
	if err != nil {
		host = requestPeer
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, network := range p.networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func requestIsHTTPS(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	value, ok := c.Get("transport-policy")
	if !ok {
		return false
	}
	policy, ok := value.(transportPolicy)
	return ok && policy.trustsPeer(c.Request.RemoteAddr) && strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https")
}

func secureSessionCookie(c *gin.Context) bool {
	if requestIsHTTPS(c) {
		return true
	}
	value, ok := c.Get("transport-policy")
	if !ok {
		return false
	}
	policy, ok := value.(transportPolicy)
	return ok && policy.secureCookies
}

func requestClientIP(c *gin.Context) string { return c.ClientIP() }

func transportSecurity(policy transportPolicy) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("transport-policy", policy)
		c.Header("Permissions-Policy", "geolocation=()")
		c.Header("Cache-Control", "no-store")
		c.Header("Content-Security-Policy", "frame-ancestors 'none'; base-uri 'self'; object-src 'none'")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		if requestIsHTTPS(c) {
			c.Header("Strict-Transport-Security", "max-age=31536000")
		}
		c.Next()
	}
}
