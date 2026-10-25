package web

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestBerryRoutesKeepMissingAssetsAndAPIErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerBerryUI(router, fstest.MapFS{
		"static/berry/index.html":              {Data: []byte("<!doctype html><script src='/next/assets/main.js'></script>")},
		"static/berry/assets/main.js":          {Data: []byte("console.log('bundle')")},
		"static/berry/assets/nested/_chunk.js": {Data: []byte("export default 1")},
		"static/berry/xunqiandao-logo.png":     {Data: []byte("image")},
	})
	router.GET("/api/test", func(c *gin.Context) { c.JSON(401, gin.H{"code": "AUTH_REQUIRED"}) })
	for _, row := range []struct {
		path     string
		status   int
		contains string
	}{
		{"/next/", 200, "<!doctype html>"}, {"/next/assets/main.js", 200, "bundle"},
		{"/next/assets/nested/_chunk.js", 200, "export default"}, {"/next/xunqiandao-logo.png", 200, "image"},
		{"/next/assets/missing.js", 404, ""}, {"/next/assets/", 404, ""}, {"/next/unknown", 404, ""},
		{"/api/test", 401, "AUTH_REQUIRED"},
	} {
		t.Run(row.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", row.path, nil))
			if w.Code != row.status || !strings.Contains(w.Body.String(), row.contains) {
				t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
			}
			if row.status != 200 && strings.Contains(w.Body.String(), "<!doctype") {
				t.Fatal("error became app HTML")
			}
		})
	}
}
func TestBerrySourceWithoutBuildReturns503(t *testing.T) {
	router := gin.New()
	registerBerryUI(router, fstest.MapFS{"static/index.html": {Data: []byte("legacy")}})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/next/", nil))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "尚未构建") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
func TestBerryDefaultAndLegacyEntry(t *testing.T) {
	router := gin.New()
	registerUIFiles(router, fstest.MapFS{
		"static/index.html":       {Data: []byte("legacy records-panel")},
		"static/calendar.js":      {Data: []byte("legacy js")},
		"static/terms.html":       {Data: []byte("terms")},
		"static/privacy.html":     {Data: []byte("privacy")},
		"static/berry/index.html": {Data: []byte("berry /next/assets/main.js")},
	})
	for _, path := range []string{"/", "/next/", "/legacy/", "/assets/calendar.js", "/terms", "/privacy"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s %d", path, w.Code)
		}
		if path == "/" && !strings.Contains(w.Body.String(), "/next/assets/") {
			t.Fatal("root is not Berry")
		}
		if path == "/legacy/" && !strings.Contains(w.Body.String(), "records-panel") {
			t.Fatal("legacy rollback unavailable")
		}
	}
}

func TestUISharedFSWithoutBerryBuild(t *testing.T) {
	router := gin.New()
	registerUIFiles(router, fstest.MapFS{
		"static/index.html":   {Data: []byte("legacy source")},
		"static/calendar.js":  {Data: []byte("legacy calendar")},
		"static/terms.html":   {Data: []byte("terms fixture")},
		"static/privacy.html": {Data: []byte("privacy fixture")},
	})
	for _, row := range []struct {
		path   string
		status int
		body   string
	}{
		{"/", 200, "legacy source"}, {"/legacy/", 200, "legacy source"}, {"/assets/calendar.js", 200, "legacy calendar"},
		{"/next/", 503, "尚未构建"}, {"/next/assets/missing.js", 404, ""},
		{"/terms", 200, "terms fixture"}, {"/api/health", 200, "ready"},
	} {
		t.Run(row.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", row.path, nil))
			if w.Code != row.status || !strings.Contains(w.Body.String(), row.body) {
				t.Fatalf("%s: %d %s", row.path, w.Code, w.Body.String())
			}
		})
	}
}

func TestUISharedFSWithBerryBuild(t *testing.T) {
	router := gin.New()
	registerUIFiles(router, fstest.MapFS{
		"static/index.html":                 {Data: []byte("legacy source")},
		"static/calendar.js":                {Data: []byte("legacy calendar")},
		"static/berry/index.html":           {Data: []byte("berry entry")},
		"static/berry/assets/deep/_part.js": {Data: []byte("export default 42")},
		"static/berry/icon.svg":             {Data: []byte("<svg></svg>")},
	})
	for _, path := range []string{"/", "/assets/calendar.js", "/next/", "/next/assets/deep/_part.js", "/next/icon.svg"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	for _, path := range []string{"/next/assets/missing.js", "/next/assets/deep/", "/next/unknown", "/next/../index.html"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
}
