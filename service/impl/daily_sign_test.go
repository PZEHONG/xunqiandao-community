package impl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

type dailyCheckerContract interface {
	HasSignInOn(context.Context, string, time.Time) (bool, error)
}

func TestDailySignCheckUsesOfficialDetailsAndShanghaiDate(t *testing.T) {
	day, _ := time.Parse(time.RFC3339, "2026-10-26T00:01:00+08:00")
	for _, tc := range []struct {
		name    string
		row     map[string]interface{}
		want    bool
		wantErr bool
	}{
		{"manual", map[string]interface{}{"sign_time_text": "2026-10-26 00:00:00", "remark_text": "下班"}, true, false},
		{"abnormal-still-exists", map[string]interface{}{"sign_time_text": "2026-10-26", "status_code": "1"}, true, false},
		{"unknown-status-still-exists", map[string]interface{}{"sign_time_text": "2026-10-26", "status_code": "99"}, true, false},
		{"seconds", map[string]interface{}{"sign_time": strconv.FormatInt(day.Unix(), 10)}, true, false},
		{"milliseconds", map[string]interface{}{"sign_time": day.UnixMilli()}, true, false},
		{"utc-prior-day-is-today-in-shanghai", map[string]interface{}{"sign_time_text": "2026-10-25T16:00:00Z"}, true, false},
		{"yesterday", map[string]interface{}{"sign_time_text": "2026-10-25 23:59:59"}, false, false},
		{"next-month", map[string]interface{}{"sign_time_text": "2026-11-01"}, false, false},
		{"invalid-date", map[string]interface{}{"sign_time_text": "2026-10-32"}, false, true},
		{"missing-date", map[string]interface{}{"status_code": "0"}, false, true},
		{"invalid-time", map[string]interface{}{"sign_time_text": "2026-10-26 99:99:99"}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := NewQueryService(&privacyRepo{}).(*QueryServiceImpl)
			checker, ok := interface{}(q).(dailyCheckerContract)
			if !ok {
				t.Fatal("official daily sign check is not implemented")
			}
			q.additionalUserData = func(string) (map[string]string, error) { return map[string]string{"school_id": "school"}, nil }
			q.client = &http.Client{Transport: reportTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Query().Get("month_date") != "2026-10" {
					t.Fatal("lost requested Shanghai month")
				}
				if req.URL.Path == "/signin40/homepage" {
					return reportResponse(`{"code":20000,"data":{"sign_in_month":[{"sign_time_text":"2026-10-26"}]}}`), nil
				}
				payload, _ := json.Marshal(map[string]interface{}{"code": 20000, "data": map[string]interface{}{"list": []interface{}{tc.row}, "sign_in_this_month": 1}})
				return reportResponse(string(payload)), nil
			})}
			got, err := checker.HasSignInOn(context.Background(), "u", day)
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("got=%t err=%v, want=%t error=%t", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestDailySignCheckEmptyPaginationFailureAndCancellation(t *testing.T) {
	day, _ := time.Parse(time.RFC3339, "2026-10-26T09:00:00+08:00")
	for _, mode := range []string{"empty", "later-page", "query-error", "missing-list", "incomplete", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			q := NewQueryService(&privacyRepo{}).(*QueryServiceImpl)
			checker, ok := interface{}(q).(dailyCheckerContract)
			if !ok {
				t.Fatal("official daily sign check is not implemented")
			}
			q.additionalUserData = func(string) (map[string]string, error) { return map[string]string{"school_id": "school"}, nil }
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			q.client = &http.Client{Transport: reportTransport(func(req *http.Request) (*http.Response, error) {
				if mode == "cancelled" {
					if req.Context().Err() == nil {
						t.Fatal("daily check lost caller cancellation")
					}
					return nil, req.Context().Err()
				}
				if req.URL.Path == "/signin40/homepage" {
					return reportResponse(`{"code":20000,"data":{}}`), nil
				}
				switch mode {
				case "query-error":
					return nil, fmt.Errorf("private-token account-address")
				case "missing-list":
					return reportResponse(`{"code":20000,"data":{}}`), nil
				case "incomplete":
					return reportResponse(`{"code":20000,"data":{"list":[],"sign_in_this_month":1}}`), nil
				case "later-page":
					if req.URL.Query().Get("page_no") == "1" {
						return reportResponse(`{"code":20000,"data":{"page_size":1,"list":[{"sign_time_text":"2026-10-25"}]}}`), nil
					}
					if req.URL.Query().Get("page_no") == "2" {
						return reportResponse(`{"code":20000,"data":{"page_size":1,"list":[{"sign_time_text":"2026-10-26"}]}}`), nil
					}
					return reportResponse(`{"code":20000,"data":{"page_size":1,"list":[],"sign_in_this_month":2}}`), nil
				default:
					return reportResponse(`{"code":20000,"data":{"list":[],"sign_in_this_month":0}}`), nil
				}
			})}
			got, err := checker.HasSignInOn(ctx, "u", day)
			wantErr := mode != "empty" && mode != "later-page"
			if got != (mode == "later-page") || (err != nil) != wantErr {
				t.Fatalf("got=%t err=%v", got, err)
			}
			if err != nil && strings.Contains(err.Error(), "private-token") {
				t.Fatal("error leaked private data")
			}
		})
	}
}
