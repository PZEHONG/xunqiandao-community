package web

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommunityAIConfigHasNoDefaultsAndNeverReturnsConnectionSettings(t *testing.T) {
	t.Setenv("REPORT_AI_CONFIG_FILE", filepath.Join(t.TempDir(), "absent.env"))
	for _, name := range []string{"REPORT_AI_BASE_URL", "REPORT_AI_MODEL", "REPORT_AI_API_KEY"} {
		t.Setenv(name, "")
	}
	base, model, key, err := loadReportAIConfig()
	if err != nil || base != "" || model != "" || key != "" {
		t.Fatal("community AI must require the deployer's own configuration")
	}
	router := authenticatedRouter(t, &Handlers{})
	for _, configured := range []bool{false, true} {
		if configured {
			t.Setenv("REPORT_AI_BASE_URL", "https://private-provider.example.invalid/v1")
			t.Setenv("REPORT_AI_MODEL", "private-fixture-model")
			t.Setenv("REPORT_AI_API_KEY", "private-fixture-credential")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/api/reports/ai/config?account=test", nil))
		var value map[string]any
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &value) != nil || len(value) != 1 || value["configured"] != configured {
			t.Fatal("AI config must only return its readiness flag")
		}
		if response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), "private-") {
			t.Fatal("connection settings leaked or were cacheable")
		}
	}
}

func TestCommunityNotificationRoutesRequireSessionAndRejectForeignUserInput(t *testing.T) {
	unauthenticated := SetupRouter(&Handlers{})
	response := httptest.NewRecorder()
	unauthenticated.ServeHTTP(response, httptest.NewRequest("GET", "/api/auth/notification-email", nil))
	if response.Code == 200 {
		t.Fatal("notification settings were exposed without authentication")
	}
	router := authenticatedRouter(t, &Handlers{})
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/api/auth/notification-email", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), "尚未绑定") {
		t.Fatal("notification routes are missing")
	}
	response = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/notification-email/enabled", strings.NewReader(`{"enabled":true,"user_id":999}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, req)
	if response.Code != 400 {
		t.Fatal("notification route accepted a client-selected user identity")
	}
}
