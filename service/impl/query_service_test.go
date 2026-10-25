package impl

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestQueryReturnsOfficialDetailsInsteadOfCalendarSummary(t *testing.T) {
	q := NewQueryService(&privacyRepo{}).(*QueryServiceImpl)
	q.additionalUserData = func(string) (map[string]string, error) { return map[string]string{"school_id": "school"}, nil }
	pages := 0
	q.client = &http.Client{Transport: reportTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/signin40/homepage" {
			return reportResponse(`{"code":20000,"data":{"continuous_sign_in":"2","sign_in_month":[{"sign_time_text":"2026-10-15","status_code":"0"}],"sign_resources_info":{"mid_sign_latitude":"23"}}}`), nil
		}
		if req.URL.Path != "/signIn40/SignInList" || req.Method != "GET" {
			t.Fatalf("unexpected request path: %s", req.URL.Path)
		}
		params := req.URL.Query()
		if params.Get("school_id") != "school" || params.Get("token") != "test-token" || params.Get("month_date") != time.Now().In(time.FixedZone("Shanghai", 28800)).Format("2006-01") {
			t.Fatal("detail request lost the connected account or current month")
		}
		pages++
		switch params.Get("page_no") {
		case "1":
			return reportResponse(`{"code":20000,"data":{"page_size":2,"sign_in_this_month":3,"list":[{"sign_time":"1792022123","sign_time_text":"2026-10-15","status_code":"0","status_code_text":"正常","remark_text":"上班","address_name":"示例园区","address":"官方完整地址"},{"status_code":"1","status_code_text":"异常","address":"第二条官方地址"}]}}`), nil
		case "2":
			return reportResponse(`{"code":20000,"data":{"page_size":2,"sign_in_this_month":3,"list":[{"status_code":"99","address_name":"第三条官方地点"}]}}`), nil
		default:
			t.Fatal("unexpected extra page")
			return nil, nil
		}
	})}
	data, err := q.QuerySignInfo("u")
	if err != nil {
		t.Fatal(err)
	}
	records, ok := data["records"].([]interface{})
	if !ok || len(records) != 3 || pages != 2 {
		t.Fatalf("expected all 3 official detail rows, got records=%v pages=%d", data["records"], pages)
	}
	first := records[0].(map[string]interface{})
	if first["address"] != "官方完整地址" || first["status_code_text"] != "正常" || first["sign_time"] != "1792022123" || data["continuous_sign_in"] != "2" {
		t.Fatal("official record values were lost or replaced")
	}
	if records[2].(map[string]interface{})["status_code_text"] != nil {
		t.Fatal("unknown official status must not be invented")
	}
	if _, ok := data["sign_resources_info"]; ok {
		t.Fatal("location configuration leaked into history")
	}
}

func TestQueryDoesNotPassOffMissingDetailsAsAnEmptyOrSuccessfulHistory(t *testing.T) {
	for _, scenario := range []string{"network", "http", "invalid-json", "upstream", "missing-list", "invalid-row", "excess", "incomplete"} {
		t.Run(scenario, func(t *testing.T) {
			q := NewQueryService(&privacyRepo{}).(*QueryServiceImpl)
			q.additionalUserData = func(string) (map[string]string, error) { return map[string]string{}, nil }
			q.client = &http.Client{Transport: reportTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/signin40/homepage" {
					return reportResponse(`{"code":20000,"data":{"sign_in_month":[{"status_code":"0"}]}}`), nil
				}
				switch scenario {
				case "network":
					return nil, fmt.Errorf("failed URL %s", req.URL.String())
				case "http":
					res := reportResponse(`{"code":20000,"data":{"list":[]}}`)
					res.StatusCode = 503
					return res, nil
				case "invalid-json":
					return reportResponse(`not json`), nil
				case "upstream":
					return reportResponse(`{"code":40001,"message":"test-token"}`), nil
				case "missing-list":
					return reportResponse(`{"code":20000,"data":{}}`), nil
				case "invalid-row":
					return reportResponse(`{"code":20000,"data":{"list":[null]}}`), nil
				case "excess":
					return reportResponse(`{"code":20000,"data":{"list":[{"status_code_text":"正常"}],"sign_in_this_month":0}}`), nil
				default:
					return reportResponse(`{"code":20000,"data":{"list":[],"sign_in_this_month":3}}`), nil
				}
			})}
			data, err := q.QuerySignInfo("u")
			if err == nil || data != nil {
				t.Fatal("unverified history was reported as successful")
			}
			if strings.Contains(err.Error(), "test-token") {
				t.Fatal("query error exposed credentials")
			}
		})
	}
}

func TestQueryAcceptsAnOfficialEmptyDetailList(t *testing.T) {
	q := NewQueryService(&privacyRepo{}).(*QueryServiceImpl)
	q.additionalUserData = func(string) (map[string]string, error) { return map[string]string{}, nil }
	q.client = &http.Client{Transport: reportTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/signin40/homepage" {
			return reportResponse(`{"code":20000,"data":{"sign_in_month":[{"status_code":"0"}]}}`), nil
		}
		return reportResponse(`{"code":20000,"data":{"list":[],"sign_in_this_month":0}}`), nil
	})}
	data, err := q.QuerySignInfo("u")
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := data["records"].([]interface{})
	if !ok || len(rows) != 0 {
		t.Fatal("empty official history must not fall back to calendar rows")
	}
}

func TestCalendarMonthIsUsedForBothSummaryAndDetails(t *testing.T) {
	q := NewQueryService(&privacyRepo{}).(*QueryServiceImpl)
	q.additionalUserData = func(string) (map[string]string, error) { return map[string]string{}, nil }
	query, ok := interface{}(q).(interface {
		QuerySignInfoForMonth(string, string) (map[string]interface{}, error)
	})
	if !ok {
		t.Fatal("monthly calendar query is not implemented")
	}
	calls := 0
	q.client = &http.Client{Transport: reportTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Query().Get("month_date") != "2026-09" {
			t.Fatalf("requested month was lost for %s", req.URL.Path)
		}
		if req.URL.Path == "/signin40/homepage" {
			return reportResponse(`{"code":20000,"data":{"continuous_sign_in":"2","sign_in_month":[]}}`), nil
		}
		return reportResponse(`{"code":20000,"data":{"list":[],"sign_in_this_month":0}}`), nil
	})}
	data, err := query.QuerySignInfoForMonth("u", "2026-09")
	if err != nil || data["month_date"] != "2026-09" || calls != 2 {
		t.Fatalf("monthly query failed: data=%v calls=%d err=%v", data, calls, err)
	}
	if _, err := query.QuerySignInfoForMonth("u", "2026-13"); err == nil || calls != 2 {
		t.Fatal("invalid month was not rejected before reaching upstream")
	}
}
