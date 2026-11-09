package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"xixunyunsign/service"
)

type reportDatesStub struct{ reportStub }

func (*reportDatesStub) List(ctx context.Context, _ string, f service.ReportFilter) (map[string]interface{}, error) {
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 25*time.Second {
		return nil, errors.New("list and detail enrichment need one bounded budget")
	}
	start, end := "2026-09-15", "2026-09-20"
	if f.BusinessType == "month" {
		start, end = "2026-09-01", "2026-09-30"
	}
	return map[string]interface{}{"count": 3, "list": []map[string]interface{}{
		{"id": "known", "start_date": start, "end_date": end, "create_time": "2026-10-14"},
		{"id": "detail", "create_time": "2026-10-14"},
		{"id": "unavailable", "create_time": "2026-10-14"},
	}}, nil
}

func TestReportRecordDateUsesBeijingForZonedTimestamps(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"2026-09-30T16:00:00Z", "2026-10-01"},
		{"2026-10-01T00:00:00+08:00", "2026-10-01"},
		{"2026-10-01T00:00:00+09:00", "2026-09-30"},
		{"2026-09-30", "2026-09-30"},
		{"2026-09-30 23:30:00", "2026-09-30"},
		{"1790784000000", "2026-10-01"},
		{"2026-02-30", ""},
	} {
		if got := reportRecordDate(test.input); got != test.want {
			t.Errorf("date(%q)=%q, want %q", test.input, got, test.want)
		}
	}
}

func (*reportDatesStub) Detail(_ context.Context, _ string, id string) (map[string]interface{}, error) {
	if id == "unavailable" {
		return nil, errors.New("detail unavailable")
	}
	if id == "known" {
		return nil, errors.New("complete dates should not need another request")
	}
	return map[string]interface{}{"start_date": "2026-09-15", "end_date": "2026-09-20", "create_time": "2026-10-14"}, nil
}

func TestReportListResolvesReportDatesWithoutUsingSubmissionTime(t *testing.T) {
	for _, kind := range []string{"week", "month"} {
		router := authenticatedRouter(t, &Handlers{ReportService: &reportDatesStub{}})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/api/reports?account=test&business_type="+kind, nil))
		var response struct {
			Data struct {
				Count int                      `json:"count"`
				List  []map[string]interface{} `json:"list"`
			} `json:"data"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Data.Count != 3 || len(response.Data.List) != 3 {
			t.Fatalf("list: %d %s", w.Code, w.Body.String())
		}
		rows := response.Data.List
		if rows[1]["start_date"] != "2026-09-15" || rows[1]["end_date"] != "2026-09-20" || rows[1]["create_time"] != "2026-10-14" {
			t.Fatalf("report dates must come from detail, preserving submission: %+v", rows[1])
		}
		if rows[0]["start_date"] == nil || rows[2]["start_date"] != nil || rows[2]["end_date"] != nil {
			t.Fatalf("complete dates lost or submission date used as fallback: %+v", rows)
		}
	}
}

type lateSubmissionStub struct{ reportStub }

func (*lateSubmissionStub) List(context.Context, string, service.ReportFilter) (map[string]interface{}, error) {
	return map[string]interface{}{"count": 1, "list": []interface{}{map[string]interface{}{"id": "late", "create_time": "2026-10-14"}}}, nil
}
func (*lateSubmissionStub) Detail(context.Context, string, string) (map[string]interface{}, error) {
	return map[string]interface{}{"start_date": "2026-09-15", "end_date": "2026-09-20", "create_time": "2026-10-14"}, nil
}
func TestAISuggestUsesReportPeriodForLateSubmission(t *testing.T) {
	for _, kind := range []string{"week", "month"} {
		history, err := aiReportHistory(context.Background(), &lateSubmissionStub{}, "test", kind)
		if err != nil {
			t.Fatal(err)
		}
		got := suggestAIDates(kind, "single", history, time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC))
		wantStart, wantEnd := "2026-09-20", "2026-09-27"
		if kind == "month" {
			wantStart, wantEnd = "2026-09-21", "2026-09-30"
		}
		if got.Start != wantStart || got.End != wantEnd || strings.Contains(got.Start, "2026-10-14") {
			t.Fatalf("%s suggestion based on wrong date: %+v", kind, got)
		}
	}
}
