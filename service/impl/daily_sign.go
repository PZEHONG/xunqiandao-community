package impl

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var signRecordZone = time.FixedZone("Asia/Shanghai", 8*60*60)

// HasSignInOn deliberately ignores source, remark and normal/abnormal status:
// any official record on the requested Shanghai day prevents another daily sign.
func (q *QueryServiceImpl) HasSignInOn(ctx context.Context, account string, day time.Time) (bool, error) {
	day = day.In(signRecordZone)
	data, err := q.querySignInfoForMonth(ctx, account, day.Format("2006-01"))
	if err != nil {
		return false, err
	}
	rows, ok := data["records"].([]interface{})
	if !ok {
		return false, fmt.Errorf("无法核验今日签到明细")
	}
	invalidDate := false
	for _, raw := range rows {
		row, ok := raw.(map[string]interface{})
		if !ok {
			return false, fmt.Errorf("无法核验今日签到明细")
		}
		date, valid := officialSignDate(row)
		if !valid {
			invalidDate = true
			continue
		}
		if date == day.Format("2006-01-02") {
			return true, nil
		}
	}
	if invalidDate {
		return false, fmt.Errorf("签到明细日期不完整，无法确认今日是否已签到")
	}
	return false, nil
}

func officialSignDate(row map[string]interface{}) (string, bool) {
	value := strings.TrimSpace(fmt.Sprint(row["sign_time"]))
	if n, ok := row["sign_time"].(float64); ok {
		value = strconv.FormatFloat(n, 'f', -1, 64)
	}
	if len(value) == 10 || len(value) == 13 {
		if n, err := strconv.ParseInt(value, 10, 64); err == nil && n > 0 {
			if len(value) == 13 {
				n /= 1000
			}
			return time.Unix(n, 0).In(signRecordZone).Format("2006-01-02"), true
		}
	}
	text, _ := row["sign_time_text"].(string)
	text = strings.TrimSpace(text)
	if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
		return parsed.In(signRecordZone).Format("2006-01-02"), true
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02T15:04:05", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, text, signRecordZone); err == nil {
			return parsed.Format("2006-01-02"), true
		}
	}
	return "", false
}
