// service/query.go
package service

import (
	"context"
	"fmt"
	"time"
)

// DailySignChecker checks official records of any source, type or result.
// An error means absence could not be verified and callers must not submit.
type DailySignChecker interface {
	HasSignInOn(context.Context, string, time.Time) (bool, error)
}

// MonthlyQueryService adds calendar month selection without changing existing callers.
type MonthlyQueryService interface {
	QuerySignInfoForMonth(account, month string) (map[string]interface{}, error)
}

func ValidateSignMonth(month string) error {
	parsed, err := time.Parse("2006-01", month)
	if err != nil || len(month) != 7 || parsed.Year() < 1000 {
		return fmt.Errorf("查询月份格式应为 YYYY-MM，且必须是有效月份")
	}
	return nil
}

// QueryService 定义查询相关功能接口
type QueryService interface {
	// QuerySignInfo 读取当月官方签到明细与统计，不保存或推断签到地点。
	QuerySignInfo(account string) (map[string]interface{}, error)
	// SearchSchool 查询学校信息（模糊匹配）
	SearchSchool(schoolName string) ([]SchoolInfo, error) // Use local service.SchoolInfo
}
