// Package bos — HTTP reverse proxy helpers for P0.
// P0: BOS proxies to MBS process over HTTP (localhost:8081 by default).
// P1: replace with gRPC via MBSCaller; APP-facing BOS routes stay unchanged.
package bos

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// MBSProxy returns a gin.HandlerFunc that reverse-proxies all requests
// under the current route group to the MBS process at mbsAddr.
//
// Strip behavior: if the BOS mounts this at  /api/v1/bos/cos/proxy/
// and APP calls                /api/v1/bos/cos/proxy/customers
// it is forwarded to  {mbsAddr}/api/v1/{mbsName}/customers
//
// i.e. the "/api/v1/bos/{bos}/proxy" prefix is replaced by "/api/v1/{mbsName}".
func MBSProxy(mbsName, mbsAddr string) gin.HandlerFunc {
	target, err := url.Parse("http://" + mbsAddr)
	if err != nil {
		// config error — fail at startup, not per-request
		panic("invalid MBS addr for proxy: " + mbsAddr)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	// Preserve original path query; Director rewrites path.
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Rewrite: /api/v1/bos/{bos}/proxy/{mbs}/xxx → /api/v1/{mbs}/xxx
		path := req.URL.Path
		marker := "/proxy/" + mbsName
		if idx := strings.Index(path, marker); idx >= 0 {
			req.URL.Path = "/api/v1/" + mbsName + path[idx+len(marker):]
		}
		req.Host = target.Host
	}

	return func(c *gin.Context) {
		proxy.ServeHTTP(c.Writer, c.Request)
	}
}

// ProxyMount returns the relative path prefix each BOS should mount its
// MBS proxy under. APP calls: BOS/{bos}/proxy/{mbs-route}.
func ProxyMount() string {
	return "/proxy/*any"
}
