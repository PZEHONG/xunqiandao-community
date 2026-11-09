package web

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestWeeklyContinuationUsesLastSubmissionEnd(t *testing.T) {
	now := time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)
	// StudentSearch is ordered by create_time.desc, not by report period.
	for _, history := range [][]aiPeriod{
		{{Start: "2026-09-15", End: "2026-09-20"}},
		screenshotReportHistory(),
	} {
		for _, mode := range []string{"single", "batch"} {
			got := suggestAIDates("week", mode, history, now)
			wantEnd := "2026-09-27"
			if mode == "batch" {
				wantEnd = "2026-10-29"
			}
			if got.Start != "2026-09-20" || got.End != wantEnd {
				t.Fatalf("%s suggestion=%+v, want 2026-09-20 to %s", mode, got, wantEnd)
			}
		}
	}
}

func TestWeeklyContinuationKeepsWholePeriodsAndSharedBoundaries(t *testing.T) {
	got, err := missingAIReports(context.Background(), &reportGapsStub{}, "test", "week", "2026-09-20", "2026-10-25")
	want := []aiPeriod{
		{Start: "2026-09-20", End: "2026-09-27", State: "missing"},
		{Start: "2026-09-27", End: "2026-10-04", State: "missing"},
		{Start: "2026-10-04", End: "2026-10-11", State: "missing"},
		{Start: "2026-10-11", End: "2026-10-18", State: "missing"},
		{Start: "2026-10-18", End: "2026-10-25", State: "missing"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("periods=%+v, error=%v; want %+v", got, err, want)
	}
}

func TestWeeklyContinuationSkipsOnlyCompleteCoveredPeriods(t *testing.T) {
	got, err := planAIReportDates("week", "2026-09-20", "2026-10-11", []aiPeriod{
		{Start: "2026-09-20", End: "2026-09-27"},
		{Start: "2026-09-28", End: "2026-10-04"},
	})
	want := []aiPeriod{
		{Start: "2026-09-20", End: "2026-09-27", State: "existing"},
		{Start: "2026-09-27", End: "2026-10-04", State: "missing"},
		{Start: "2026-10-04", End: "2026-10-11", State: "missing"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("periods=%+v, error=%v; want %+v", got, err, want)
	}
}

func TestWeeklyContinuationEndsOnBeijingToday(t *testing.T) {
	for _, tt := range []struct{ now, end string }{
		{"2026-09-26T15:59:59Z", "2026-09-26"},
		{"2026-09-26T16:00:00Z", "2026-09-27"},
		{"2026-10-03T15:59:59Z", "2026-10-03"},
		{"2026-10-03T16:00:00Z", "2026-10-04"},
	} {
		now, _ := time.Parse(time.RFC3339, tt.now)
		got := suggestAIDates("week", "batch", []aiPeriod{{Start: "2026-09-15", End: "2026-09-20"}}, now)
		if got.End != tt.end || (tt.end == "" && got.Start != "") {
			t.Fatalf("now=%s suggestion=%+v, want end=%s", tt.now, got, tt.end)
		}
	}
}

func TestWeeklyBatchIncludesCurrentPartialPeriodThroughToday(t *testing.T) {
	now := time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)
	history := []aiPeriod{{Start: "2026-09-15", End: "2026-09-20"}}
	dates := suggestAIDates("week", "batch", history, now)
	periods, err := planAIReportDates("week", dates.Start, dates.End, history)
	want := []aiPeriod{
		{Start: "2026-09-20", End: "2026-09-27", State: "missing"},
		{Start: "2026-09-27", End: "2026-10-04", State: "missing"},
		{Start: "2026-10-04", End: "2026-10-11", State: "missing"},
		{Start: "2026-10-11", End: "2026-10-18", State: "missing"},
		{Start: "2026-10-18", End: "2026-10-25", State: "missing"},
		{Start: "2026-10-25", End: "2026-10-29", State: "missing"},
	}
	if err != nil || !reflect.DeepEqual(periods, want) {
		t.Fatalf("periods=%+v, error=%v; want %+v", periods, err, want)
	}
}
