package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"xixunyunsign/service"
)

type calendarQueryStub struct {
	month         string
	defaultCalled bool
}

func (q *calendarQueryStub) QuerySignInfo(string) (map[string]interface{}, error) {
	q.defaultCalled = true
	return map[string]interface{}{"records": []interface{}{}}, nil
}

func (q *calendarQueryStub) QuerySignInfoForMonth(account, month string) (map[string]interface{}, error) {
	q.month = month
	return map[string]interface{}{"month_date": month, "records": []interface{}{}}, nil
}

func (*calendarQueryStub) SearchSchool(string) ([]service.SchoolInfo, error) { return nil, nil }

func TestCalendarQuerySelectsRequestedMonth(t *testing.T) {
	q := &calendarQueryStub{}
	r := authenticatedRouter(t, &Handlers{QueryService: q})
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/query?account=test&month_date=2026-09", nil))
	if response.Code != http.StatusOK || q.month != "2026-09" || q.defaultCalled {
		t.Fatalf("month selection lost: status=%d month=%q default=%v", response.Code, q.month, q.defaultCalled)
	}
}

func TestCalendarQueryRejectsInvalidMonthBeforeQuerying(t *testing.T) {
	for _, month := range []string{"2026-13", "2026-1", "2026-09-01", "invalid"} {
		q := &calendarQueryStub{}
		response := httptest.NewRecorder()
		authenticatedRouter(t, &Handlers{QueryService: q}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/query?account=test&month_date="+month, nil))
		if response.Code != http.StatusBadRequest || q.month != "" || q.defaultCalled {
			t.Fatalf("invalid month %q reached the query service: status=%d", month, response.Code)
		}
	}
}
