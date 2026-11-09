package web

import (
	"context"
	"reflect"
	"testing"
	"time"
	"xixunyunsign/service"
)

// Match the user's seven report dates, including the two shortened weeks.
func screenshotReportHistory() []aiPeriod {
	return []aiPeriod{
		{Start: "2026-09-15", End: "2026-09-20"},
		{Start: "2026-09-21", End: "2026-09-27"},
		{Start: "2026-09-28", End: "2026-10-04"},
		{Start: "2026-10-05", End: "2026-10-11"},
		{Start: "2026-10-12", End: "2026-10-14"},
		{Start: "2026-09-07", End: "2026-09-13"},
		{Start: "2026-09-01", End: "2026-09-06"},
	}
}

type reportGapsStub struct{ reportStub }

func (*reportGapsStub) List(context.Context, string, service.ReportFilter) (map[string]interface{}, error) {
	rows := []map[string]interface{}{}
	for i, p := range screenshotReportHistory() {
		rows = append(rows, map[string]interface{}{"id": i + 1, "start_date": p.Start, "end_date": p.End, "create_time": "2026-10-14"})
	}
	return map[string]interface{}{"count": len(rows), "list": rows}, nil
}

func TestAIWeeklyBatchEndsOnUseDate(t *testing.T) {
	for _, test := range []struct {
		name, now  string
		history    []aiPeriod
		start, end string
	}{
		{"screenshot Thursday", "2026-10-29T08:00:00Z", screenshotReportHistory(), "2026-09-20", "2026-10-29"},
		{"no history", "2026-10-29T08:00:00Z", nil, "2026-10-25", "2026-10-29"},
		{"last day included in Beijing", "2026-10-24T16:01:00Z", nil, "2026-10-25", "2026-10-25"},
		{"first day new week", "2026-10-26T08:00:00Z", nil, "2026-10-25", "2026-10-26"},
		{"historical Thursday cadence", "2026-10-29T08:00:00Z", []aiPeriod{{Start: "2026-10-01", End: "2026-10-07"}}, "2026-10-07", "2026-10-29"},
		{"cross year", "2027-01-02T08:00:00Z", nil, "2026-12-27", "2027-01-02"},
		{"already complete", "2026-10-29T08:00:00Z", []aiPeriod{{Start: "2026-10-19", End: "2026-10-25"}, {Start: "2026-10-26", End: "2026-10-28"}}, "2026-10-25", "2026-10-29"},
		{"only unfinished history", "2026-10-29T08:00:00Z", []aiPeriod{{Start: "2026-10-26", End: "2026-10-28"}}, "2026-10-28", "2026-10-29"},
		{"already through today", "2026-10-29T08:00:00Z", []aiPeriod{{Start: "2026-10-25", End: "2026-10-29"}}, "2026-10-29", "2026-10-29"},
	} {
		t.Run(test.name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, test.now)
			if err != nil {
				t.Fatal(err)
			}
			got := suggestAIDates("week", "batch", test.history, now)
			if got.Start != test.start || got.End != test.end {
				t.Fatalf("got %+v, want %s to %s", got, test.start, test.end)
			}
			if got.Start != "" {
				periods, err := aiPeriods("week", got.Start, got.End)
				if err != nil || len(periods) == 0 {
					t.Fatalf("invalid suggestion: %+v %v", got, err)
				}
			}
		})
	}
}

func TestAIMonthlyGapBoundariesAndCoverage(t *testing.T) {
	for _, test := range []struct {
		name, kind, start, end string
		history                []aiPeriod
		missing                []aiPeriod
	}{
		{"monthly partial reports", "month", "2024-02-01", "2024-03-02", []aiPeriod{{Start: "2024-02-01", End: "2024-02-05"}, {Start: "2024-02-10", End: "2024-02-28"}}, []aiPeriod{{Start: "2024-02-06", End: "2024-02-09", State: "missing"}, {Start: "2024-02-29", End: "2024-02-29", State: "missing"}, {Start: "2024-03-01", End: "2024-03-02", State: "missing"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			periods, err := planAIReportDates(test.kind, test.start, test.end, test.history)
			if err != nil {
				t.Fatal(err)
			}
			var missing []aiPeriod
			cursor, _ := time.Parse("2006-01-02", test.start)
			for _, period := range periods {
				if period.Start != cursor.Format("2006-01-02") || period.End < period.Start {
					t.Fatalf("partition has skipped or duplicated dates: %+v", periods)
				}
				for cursor.Format("2006-01-02") <= period.End {
					covered := false
					for _, row := range test.history {
						date := cursor.Format("2006-01-02")
						covered = covered || row.Start <= date && date <= row.End
					}
					if covered != (period.State == "existing") {
						t.Fatalf("wrong coverage on %s: %+v", cursor, period)
					}
					cursor = cursor.AddDate(0, 0, 1)
				}
				if period.State == "missing" {
					missing = append(missing, period)
				}
			}
			if cursor.AddDate(0, 0, -1).Format("2006-01-02") != test.end || !reflect.DeepEqual(missing, test.missing) {
				t.Fatalf("missing=%+v, want %+v; periods=%+v", missing, test.missing, periods)
			}
		})
	}
}
