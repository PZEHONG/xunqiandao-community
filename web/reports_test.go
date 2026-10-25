package web

import (
	"context"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"xixunyunsign/service"
)

type reportStub struct{ last service.ReportSubmission }

func (*reportStub) List(context.Context, string, service.ReportFilter) (map[string]interface{}, error) {
	return map[string]interface{}{"count": 0, "list": []interface{}{}}, nil
}
func (*reportStub) Template(context.Context, string, string) ([]service.ReportField, error) {
	return []service.ReportField{{Title: "工作情况", Require: "1", Sort: 1}}, nil
}
func (*reportStub) Detail(context.Context, string, string) (map[string]interface{}, error) {
	return map[string]interface{}{"id": "42"}, nil
}
func (s *reportStub) Submit(_ context.Context, _ string, v service.ReportSubmission) (map[string]interface{}, error) {
	s.last = v
	return map[string]interface{}{}, nil
}
func (*reportStub) Upload(context.Context, string, string, io.Reader) (string, error) {
	return "/upload/file.txt", nil
}
func TestReportRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &reportStub{}
	router := authenticatedRouter(t, &Handlers{ReportService: stub})
	for _, path := range []string{"/api/reports?account=test&business_type=week", "/api/reports/template?account=test&business_type=month", "/api/reports/detail?account=test&id=42"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
	body := `{"account":"test","business_type":"week","start_date":"2026-10-01","end_date":"2026-10-07","content":[{"title":"工作情况","content":"正文","require":"1","sort":1}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/reports", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != 200 || stub.last.BusinessType != "week" {
		t.Fatalf("submit status=%d input=%v", w.Code, stub.last)
	}
}
func TestReportRoutesRejectMissingParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := authenticatedRouter(t, &Handlers{ReportService: &reportStub{}})
	for _, path := range []string{"/api/reports", "/api/reports/template", "/api/reports/detail?account=test", "/api/reports?account=test&business_type=week&page=not-a-number"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/api/reports", strings.NewReader("{}")))
	if w.Code != 400 {
		t.Fatalf("empty submit status=%d", w.Code)
	}
}
