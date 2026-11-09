package web

import (
	"sort"
	"time"
)

func aiCycleEnd(kind string, date time.Time, weekday time.Weekday) time.Time {
	if kind == "month" {
		return time.Date(date.Year(), date.Month()+1, 0, 0, 0, 0, 0, time.UTC)
	}
	return date.AddDate(0, 0, (int(weekday)+6-int(date.Weekday())+7)%7)
}

// Weekly intervals retain shared boundaries; monthly gaps use daily coverage.
func planAIReportDates(kind, start, end string, history []aiPeriod) ([]aiPeriod, error) {
	cycles, err := aiPeriods(kind, start, end)
	if err != nil {
		return nil, err
	}
	if kind == "week" {
		// A weekly report is one complete interval, with the next interval
		// starting on its end date. Partial overlaps never split a week.
		for i := range cycles {
			for _, row := range history {
				if row.Start <= cycles[i].Start && row.End >= cycles[i].End {
					cycles[i].State = "existing"
					break
				}
			}
		}
		return cycles, nil
	}
	a, _ := time.Parse("2006-01-02", start)
	b, _ := time.Parse("2006-01-02", end)
	rows := append([]aiPeriod(nil), history...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Start < rows[j].Start })
	periods := []aiPeriod{}
	next, coveredUntil := 0, ""
	for day := a; !day.After(b); day = day.AddDate(0, 0, 1) {
		date := day.Format("2006-01-02")
		for next < len(rows) && rows[next].Start <= date {
			if rows[next].End > coveredUntil {
				coveredUntil = rows[next].End
			}
			next++
		}
		state := "missing"
		if date <= coveredUntil {
			state = "existing"
		}
		boundary := day.Day() == 1
		if len(periods) == 0 || boundary || periods[len(periods)-1].State != state {
			periods = append(periods, aiPeriod{Start: date, End: date, State: state})
		} else {
			periods[len(periods)-1].End = date
		}
	}
	return periods, nil
}

func suggestAIDates(kind, mode string, history []aiPeriod, now time.Time) aiDateSuggestion {
	today, _ := time.Parse("2006-01-02", now.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02"))
	if kind == "week" {
		return suggestWeeklyDates(mode, history, today)
	}
	currentStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	first, through := currentStart, today
	if mode == "single" {
		through = aiCycleEnd(kind, today, time.Monday)
	}
	rows := append([]aiPeriod(nil), history...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Start < rows[j].Start })
	if len(rows) > 0 && rows[0].Start <= today.Format("2006-01-02") {
		first, _ = time.Parse("2006-01-02", rows[0].Start)
	}
	// Match the public planner's three-year limit, including single-period ends.
	if floor := through.AddDate(0, 0, -3*366); first.Before(floor) {
		first = floor
	}
	periods, _ := planAIReportDates(kind, first.Format("2006-01-02"), through.Format("2006-01-02"), rows)
	for _, period := range periods {
		if period.State != "missing" {
			continue
		}
		end := period.End
		if mode == "batch" {
			end = through.Format("2006-01-02")
		}
		reason := "已按报告日期找到最早未覆盖日期，短缺口会单独补全。"
		if len(rows) == 0 {
			reason = "暂无历史报告，已填写当前周期，可自行调整。"
		}
		return aiDateSuggestion{Start: period.Start, End: end, Reason: reason}
	}
	if mode == "batch" {
		return aiDateSuggestion{Start: currentStart.Format("2006-01-02"), End: through.Format("2006-01-02"), Reason: "已有报告覆盖至当前周期，可调整日期检查更早的缺报。"}
	}
	latestEnd := through.Format("2006-01-02")
	for _, row := range rows {
		if row.End > latestEnd {
			latestEnd = row.End
		}
	}
	last, _ := time.Parse("2006-01-02", latestEnd)
	first = last.AddDate(0, 0, 1)
	return aiDateSuggestion{Start: first.Format("2006-01-02"), End: aiCycleEnd(kind, first, time.Monday).Format("2006-01-02"), Reason: "当前周期已有报告，已建议下一周期，可自行调整。"}
}

func suggestWeeklyDates(mode string, history []aiPeriod, today time.Time) aiDateSuggestion {
	// List order is create_time.desc across all pages. Preserve that order:
	// the last submission can be a shortened or backfilled report.
	first := today.AddDate(0, 0, -int(today.Weekday()))
	reason := "暂无历史周报，已按周日衔接填写日期，可自行调整。"
	if len(history) > 0 {
		first, _ = time.Parse("2006-01-02", history[0].End)
		reason = "已从最后一次提交周报的截止日衔接，每次向后推七天。"
	}
	through := first.AddDate(0, 0, 7)
	if mode == "batch" {
		through = today
		if first.After(today) {
			return aiDateSuggestion{Start: today.Format("2006-01-02"), End: today.Format("2006-01-02"), Reason: "最后提交周报的截止日晚于今天，已将日期填写为今天，可手动调整起始日期。"}
		}
		// Keep long histories within the planner limit without shifting cadence.
		if through.Sub(first) > 3*366*24*time.Hour {
			days := int(through.Sub(first) / (24 * time.Hour))
			first = first.AddDate(0, 0, ((days-3*366+6)/7)*7)
		}
		reason += "截止日期已按北京时间填写为今天，最后一段补全至今天。"
	}
	return aiDateSuggestion{Start: first.Format("2006-01-02"), End: through.Format("2006-01-02"), Reason: reason}
}
