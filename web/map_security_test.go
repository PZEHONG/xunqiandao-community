package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"xixunyunsign/platformauth"

	"github.com/gin-gonic/gin"
)

type mapRoundTripFunc func(*http.Request) (*http.Response, error)

func (f mapRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMapProxyAuthenticatedFullRouterUsesFixedUpstream(t *testing.T) {
	t.Setenv("AMAP_CONFIG_FILE", t.TempDir()+"/absent")
	t.Setenv("AMAP_JS_KEY", "test-js-key")
	t.Setenv("AMAP_SECURITY_JS_CODE", "test-private-code")
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	var calls int
	http.DefaultTransport = mapRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Scheme != "https" || r.URL.Host != "restapi.amap.com" || r.URL.Path != "/v4/maps" {
			t.Errorf("incorrect SDK origin: %s", r.URL)
		}
		if r.URL.Query().Get("key") != "test-js-key" || r.URL.Query().Get("jscode") != "test-private-code" {
			t.Error("credentials not replaced")
		}
		if r.URL.Query().Get("callback") != "AMap._cb1" {
			t.Error("SDK JSONP callback was not preserved")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/javascript"}}, Body: io.NopCloser(strings.NewReader(`AMap._cb1({"ok":true})`))}, nil
	})
	w := mapSecurityRequest(authenticatedRouter(t, &Handlers{}), "/_AMapService/v4/maps?callback=AMap._cb1&key=untrusted&jscode=untrusted", "192.0.2.1")
	if w.Code != 200 || calls != 1 || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("authenticated SDK request=%d calls=%d body=%s", w.Code, calls, w.Body)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing content-type protection")
	}
}

func TestMapProxyNeverFollowsProviderRedirects(t *testing.T) {
	var calls int
	h := &locationHandler{config: func() (amapConfig, error) { return amapConfig{JSKey: "fake-key", SecurityCode: "fake-code"}, nil }, client: &http.Client{Transport: mapRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://other.example/collect"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}}
	w := mapSecurityRequest(mapSecurityRouter(h, 1), "/_AMapService/v4/maps", "192.0.2.1")
	if w.Code != 502 || calls != 1 {
		t.Fatalf("redirect handling status=%d calls=%d", w.Code, calls)
	}
}

func TestMapProxyResponseBoundAndQuotaReleaseAfterFailure(t *testing.T) {
	var calls int
	h := testAMapHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			io.WriteString(w, strings.Repeat("a", (8<<20)+1))
			return
		}
		io.WriteString(w, `{}`)
	})
	r := mapSecurityRouter(h, 1)
	if w := mapSecurityRequest(r, "/_AMapService/v4/maps", "192.0.2.1"); w.Code != 502 || w.Body.Len() > 1024 {
		t.Fatalf("oversized response exposed: status=%d bytes=%d", w.Code, w.Body.Len())
	}
	if w := mapSecurityRequest(r, "/_AMapService/v4/maps", "192.0.2.1"); w.Code != 200 {
		t.Fatalf("provider failure did not release quota: %d", w.Code)
	}
}

func TestMapProxyIPAndGlobalConcurrentBudgets(t *testing.T) {
	for _, tc := range []struct {
		name     string
		limit    int
		singleIP bool
	}{{"ip", 16, true}, {"global", 32, false}} {
		t.Run(tc.name, func(t *testing.T) {
			entered := make(chan struct{}, tc.limit+1)
			release := make(chan struct{})
			h := testAMapHandler(t, func(w http.ResponseWriter, _ *http.Request) {
				entered <- struct{}{}
				<-release
				io.WriteString(w, `{}`)
			})
			defer close(release)
			for i := 1; i <= tc.limit; i++ {
				ip := "192.0.2.1"
				if !tc.singleIP {
					ip = "192.0.2." + strconv.Itoa(i)
				}
				r := mapSecurityRouter(h, int64(i))
				go mapSecurityRequest(r, "/_AMapService/v4/maps", ip)
				select {
				case <-entered:
				case <-time.After(3 * time.Second):
					t.Fatal("request did not reach provider")
				}
			}
			ip := "192.0.2.1"
			if !tc.singleIP {
				ip = "192.0.2.250"
			}
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() { done <- mapSecurityRequest(mapSecurityRouter(h, 99), "/_AMapService/v4/maps", ip) }()
			select {
			case w := <-done:
				if w.Code != 429 {
					t.Fatalf("quota status=%d", w.Code)
				}
			case <-time.After(200 * time.Millisecond):
				t.Fatal("over-quota request reached provider")
			}
		})
	}
}

func TestMapBudgetRefillsAndCapsIdentityMemory(t *testing.T) {
	now := time.Unix(1700000000, 0)
	l := &mapRequestLimiter{}
	for i := 0; i < 60; i++ {
		release, ok := l.acquire(1, "192.0.2.1", now)
		if !ok {
			t.Fatal("initial SDK burst rejected")
		}
		release()
	}
	if _, ok := l.acquire(1, "192.0.2.1", now); ok {
		t.Fatal("exhausted user quota admitted")
	}
	if release, ok := l.acquire(1, "192.0.2.1", now.Add(500*time.Millisecond)); !ok {
		t.Fatal("quota did not refill")
	} else {
		release()
	}
	for i := 2; i <= 10000; i++ {
		release, ok := l.acquire(int64(i), strconv.Itoa(i), now)
		if !ok {
			t.Fatal("identity capacity rejected early")
		}
		release()
	}
	if _, ok := l.acquire(10001, "new-ip", now); ok {
		t.Fatal("identity table allowed unbounded growth")
	}
	if release, ok := l.acquire(10001, "new-ip", now.Add(6*time.Minute)); !ok {
		t.Fatal("expired entries were not reclaimed")
	} else {
		release()
	}
}

func mapSecurityRouter(h *locationHandler, user int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.SetTrustedProxies(nil)
	r.Use(func(c *gin.Context) { c.Set("platform-user", platformauth.User{ID: user}) })
	r.GET("/_AMapService/*path", h.proxy)
	return r
}

func mapSecurityRequest(r http.Handler, path, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.RemoteAddr = ip + ":12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestMapProxyFullRouterRejectsAnonymous(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("AMAP_CONFIG_FILE", t.TempDir()+"/absent")
	t.Setenv("AMAP_JS_KEY", "")
	t.Setenv("AMAP_SECURITY_JS_CODE", "")
	s, _ := newAuthFixture(t)
	r := SetupRouter(&Handlers{PlatformAuth: s})
	if w := mapSecurityRequest(r, "/_AMapService/v4/maps", "192.0.2.1"); w.Code != 401 {
		t.Fatalf("anonymous map proxy status=%d; want 401", w.Code)
	}
}

func TestMapProxyRateLimitTracksUserAcrossIPs(t *testing.T) {
	var calls atomic.Int64
	h := testAMapHandler(t, func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); io.WriteString(w, `{}`) })
	r := mapSecurityRouter(h, 7)
	var denied bool
	for i := 0; i < 80; i++ {
		ip := "192.0.2.1"
		if i%2 == 0 {
			ip = "192.0.2.2"
		}
		w := mapSecurityRequest(r, "/_AMapService/v4/maps", ip)
		if w.Code == 429 {
			denied = true
			if w.Header().Get("Retry-After") == "" {
				t.Fatal("missing retry hint")
			}
			break
		}
		if w.Code != 200 {
			t.Fatalf("authenticated map request=%d", w.Code)
		}
	}
	if !denied || calls.Load() >= 80 {
		t.Fatal("per-user quota never rejected the upstream call")
	}
	if w := mapSecurityRequest(mapSecurityRouter(h, 8), "/_AMapService/v4/maps", "192.0.2.3"); w.Code != 200 {
		t.Fatalf("unrelated user was throttled: %d", w.Code)
	}
}

func TestMapProxyRateLimitTracksIPAcrossUsers(t *testing.T) {
	h := testAMapHandler(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	for i := 1; i <= 140; i++ {
		req := httptest.NewRequest("GET", "/_AMapService/v4/maps", nil)
		req.RemoteAddr = "192.0.2.1:12345"
		req.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(i))
		w := httptest.NewRecorder()
		mapSecurityRouter(h, int64(i)).ServeHTTP(w, req)
		if w.Code == 429 {
			return
		}
		if w.Code != 200 {
			t.Fatalf("authenticated request=%d", w.Code)
		}
	}
	t.Fatal("same-IP quota could be bypassed using distinct users")
}

func TestMapProxyAndLocationLookupsShareQuota(t *testing.T) {
	var calls atomic.Int64
	h := testAMapHandler(t, func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); io.WriteString(w, `{}`) })
	r := mapSecurityRouter(h, 1)
	r.POST("/search", h.withMapLimits(h.search))
	r.POST("/reverse", h.withMapLimits(h.reverse))
	for i := 0; i < 60; i++ {
		if w := mapSecurityRequest(r, "/_AMapService/v4/maps", "192.0.2.1"); w.Code != 200 {
			t.Fatalf("burst request=%d", w.Code)
		}
	}
	for _, path := range []string{"/search", "/reverse"} {
		req := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
		req.RemoteAddr = "192.0.2.1:12345"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 429 {
			t.Fatalf("alternate location route %s bypassed quota: %d", path, w.Code)
		}
	}
	if calls.Load() != 60 {
		t.Fatalf("over-quota requests reached provider: %d", calls.Load())
	}
}

func TestMapProxyConcurrencyQuotaAndRelease(t *testing.T) {
	entered := make(chan struct{}, 10)
	release := make(chan struct{})
	h := testAMapHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		entered <- struct{}{}
		<-release
		io.WriteString(w, `{}`)
	})
	r := mapSecurityRouter(h, 1)
	done := make(chan *httptest.ResponseRecorder, 8)
	var released bool
	defer func() {
		if !released {
			close(release)
		}
	}()
	for i := 0; i < 8; i++ {
		go func() { done <- mapSecurityRequest(r, "/_AMapService/v4/maps", "192.0.2.1") }()
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("request did not reach provider")
		}
	}
	ninth := make(chan *httptest.ResponseRecorder, 1)
	go func() { ninth <- mapSecurityRequest(r, "/_AMapService/v4/maps", "192.0.2.2") }()
	select {
	case w := <-ninth:
		if w.Code != 429 {
			t.Fatalf("concurrent quota=%d; want 429", w.Code)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("ninth upstream request was allowed to block instead of rejecting")
	}
	close(release)
	released = true
	for i := 0; i < 8; i++ {
		select {
		case w := <-done:
			if w.Code != 200 {
				t.Fatalf("admitted request=%d", w.Code)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("admitted request did not finish")
		}
	}
	if w := mapSecurityRequest(r, "/_AMapService/v4/maps", "192.0.2.1"); w.Code != 200 {
		t.Fatalf("completed requests did not release quota: %d", w.Code)
	}
}

func TestMapProxyRejectsUnsafeCallbackAndOversizedQuery(t *testing.T) {
	h := testAMapHandler(t, func(http.ResponseWriter, *http.Request) { t.Error("unsafe query reached provider") })
	r := mapSecurityRouter(h, 1)
	for _, path := range []string{"/_AMapService/v4/maps?callback=alert(1)//", "/_AMapService/v4/maps?q=" + strings.Repeat("a", 17000)} {
		if w := mapSecurityRequest(r, path, "192.0.2.1"); w.Code != 400 {
			t.Fatalf("unsafe query status=%d", w.Code)
		}
	}
}
