package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHomePageAvailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := authenticatedRouter(t, &Handlers{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("homepage status = %d; want 200", response.Code)
	}
	if !strings.Contains(response.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("homepage should be HTML, got %q", response.Header().Get("Content-Type"))
	}
}

func TestUIAssetsAvailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := authenticatedRouter(t, &Handlers{})
	for _, path := range []string{"/assets/app.css", "/assets/app.js"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusOK || response.Body.Len() == 0 {
				t.Fatalf("asset %s unavailable: status=%d size=%d", path, response.Code, response.Body.Len())
			}
		})
	}
}

func TestServiceHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	authenticatedRouter(t, &Handlers{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("service health status=%d; want 200", response.Code)
	}
}

func TestInvalidAPIRequestsStillReturnJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := authenticatedRouter(t, &Handlers{})
	for _, testCase := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/query", ""},
		{http.MethodGet, "/api/search_school", ""},
		{http.MethodPost, "/api/login", "{}"},
		{http.MethodPost, "/api/sign", "{}"},
	} {
		t.Run(testCase.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("invalid API request status=%d; want 400", response.Code)
			}
			if !strings.Contains(response.Header().Get("Content-Type"), "application/json") {
				t.Fatal("API response changed from JSON to HTML")
			}
		})
	}
}
