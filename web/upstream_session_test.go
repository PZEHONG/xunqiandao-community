package web

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"xixunyunsign/service"
)

type expiredReportStub struct{ reportStub }

func (*expiredReportStub) List(context.Context, string, service.ReportFilter) (map[string]interface{}, error) {
	return nil, service.ErrUpstreamLoginRequired
}

func TestUpstreamExpiryRequestsReconnectWithoutRevokingLocalSession(t *testing.T) {
	router := authenticatedRouter(t, &Handlers{ReportService: &expiredReportStub{}})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/reports?account=test&business_type=month", nil))
	var result struct {
		Code string `json:"code"`
	}
	if w.Code != 401 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Code != "UPSTREAM_LOGIN_REQUIRED" {
		t.Fatal("upstream and local expiration confused", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/auth/self", nil))
	if w.Code != 200 {
		t.Fatal("local session was invalidated", w.Code)
	}
}
