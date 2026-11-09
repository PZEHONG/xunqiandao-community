package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"
	"xixunyunsign/service"
)

func reportRecordDate(value interface{}) string {
	s := fmt.Sprint(value)
	beijing := time.FixedZone("CST", 8*3600)
	if len(s) == 10 {
		if _, err := time.Parse("2006-01-02", s); err == nil {
			return s
		}
	}
	if date, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return date.In(beijing).Format("2006-01-02")
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if date, err := time.ParseInLocation(layout, s, beijing); err == nil {
			return date.Format("2006-01-02")
		}
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n <= 0 || math.IsNaN(n) || math.IsInf(n, 0) {
		return ""
	}
	if n >= 1e12 {
		n /= 1000
	}
	date := time.Unix(int64(n), 0).In(beijing)
	if date.Year() < 1 || date.Year() > 9999 {
		return ""
	}
	return date.Format("2006-01-02")
}

// Lists and AI planning share report dates; create_time is only a submission timestamp.
func reportDates(ctx context.Context, reports service.ReportService, account string, row map[string]interface{}) (string, string, error) {
	a, b := reportRecordDate(row["start_date"]), reportRecordDate(row["end_date"])
	if a != "" && b != "" && a <= b {
		return a, b, nil
	}
	id := fmt.Sprint(row["id"])
	if id != "" && id != "<nil>" {
		detail, err := reports.Detail(ctx, account, id)
		if err != nil {
			return "", "", err
		}
		a, b = reportRecordDate(detail["start_date"]), reportRecordDate(detail["end_date"])
		if a != "" && b != "" && a <= b {
			return a, b, nil
		}
	}
	return "", "", errors.New("部分报告缺少有效的报告日期，请查看详情后核对，暂不自动判断缺报。")
}

// Bound both concurrent detail requests and total wait; keep the list readable on partial failures.
func enrichReportDates(ctx context.Context, reports service.ReportService, account string, data map[string]interface{}) {
	raw, err := json.Marshal(data["list"])
	if err != nil {
		return
	}
	var rows []map[string]interface{}
	if json.Unmarshal(raw, &rows) != nil || rows == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var workers sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		workers.Add(1)
		go func(offset int) {
			defer workers.Done()
			for i := offset; i < len(rows); i += 4 {
				row := rows[i]
				if row == nil {
					continue
				}
				a, b, err := reportDates(ctx, reports, account, row)
				if err != nil {
					delete(row, "start_date")
					delete(row, "end_date")
					continue
				}
				row["start_date"], row["end_date"] = a, b
			}
		}(worker)
	}
	workers.Wait()
	data["list"] = rows
}
