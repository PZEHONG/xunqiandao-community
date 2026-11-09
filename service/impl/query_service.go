// service/impl/query_service.go
package impl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"xixunyunsign/service"
	"xixunyunsign/utils" // Keep for GetAdditionalUserData for now, ideally refactor that too
)

// QueryServiceImpl implements the QueryService interface.
type QueryServiceImpl struct {
	userRepo           service.UserRepository // Add UserRepository dependency
	client             *http.Client
	additionalUserData func(string) (map[string]string, error)
	// httpClient *http.Client      // Consider injecting http client if needed for external calls
}

// NewQueryService creates a new QueryService implementation.
func NewQueryService(repo service.UserRepository) service.QueryService {
	// Return the exported type
	return &QueryServiceImpl{
		userRepo:           repo,
		client:             &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		additionalUserData: utils.GetAdditionalUserData,
		// httpClient: &http.Client{}, // Initialize http client if needed
	}
}

// QuerySignInfo retrieves sign-in information for a user.
func (q *QueryServiceImpl) QuerySignInfo(account string) (map[string]interface{}, error) {
	return q.QuerySignInfoForMonth(account, time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("2006-01"))
}

// QuerySignInfoForMonth uses the same requested month for calendar summaries and all detail pages.
func (q *QueryServiceImpl) QuerySignInfoForMonth(account, month string) (map[string]interface{}, error) {
	return q.querySignInfoForMonth(context.Background(), account, month)
}

func (q *QueryServiceImpl) querySignInfoForMonth(ctx context.Context, account, month string) (map[string]interface{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := service.ValidateSignMonth(month); err != nil {
		return nil, err
	}
	// Use injected userRepo
	token, _, _, err := q.userRepo.GetUser(account)
	if err != nil || token == "" {
		return nil, fmt.Errorf("账号连接已失效，请前往账号设置重新连接")
	}

	// TODO: Refactor GetAdditionalUserData to be part of UserRepository or another service
	userData, err := q.additionalUserData(account)
	if err != nil {
		return nil, fmt.Errorf("无法读取学校信息，请重新连接账号")
	}

	qry := make(url.Values)
	qry.Add("month_date", month)
	qry.Add("token", token)
	qry.Add("from", "app")
	qry.Add("version", "5.1.3")
	qry.Add("platform", "android")
	qry.Add("entrance_year", userData["entrance_year"])
	qry.Add("graduate_year", userData["graduation_year"])
	qry.Add("school_id", userData["school_id"])
	data, err := q.queryDataContext(ctx, account, "signin40/homepage", qry)
	if err != nil {
		return nil, err
	}
	// Homepage rows are calendar summaries. Never use them as location history.
	delete(data, "sign_resources_info")
	records := make([]interface{}, 0)
	qry.Set("page_size", "100")
	for page := 1; page <= 50; page++ {
		qry.Set("page_no", strconv.Itoa(page))
		detail, err := q.queryDataContext(ctx, account, "signIn40/SignInList", qry)
		if err != nil {
			return nil, err
		}
		rows, ok := detail["list"].([]interface{})
		if !ok {
			return nil, fmt.Errorf("习讯云未返回有效的签到明细，请稍后刷新")
		}
		for _, row := range rows {
			if _, ok := row.(map[string]interface{}); !ok {
				return nil, fmt.Errorf("习讯云返回的签到明细不完整，请稍后刷新")
			}
		}
		records = append(records, rows...)
		pageSize := 100
		if size, err := strconv.Atoi(fmt.Sprint(detail["page_size"])); err == nil && size > 0 && size <= 100 {
			pageSize = size
		}
		if len(rows) < pageSize {
			if total, err := strconv.Atoi(fmt.Sprint(detail["sign_in_this_month"])); err == nil && total != len(records) {
				return nil, fmt.Errorf("习讯云返回的签到明细与总数不一致，请重新刷新核对")
			}
			data["records"] = records
			data["month_date"] = month
			data["sign_count"] = len(records)
			return data, nil
		}
	}
	return nil, fmt.Errorf("签到记录较多，暂未能完整读取，请稍后刷新或前往习讯云查看")
}

// queryData keeps transport URLs (which contain credentials) out of user errors.
func (q *QueryServiceImpl) queryData(account, path string, qry url.Values) (map[string]interface{}, error) {
	return q.queryDataContext(context.Background(), account, path, qry)
}

func (q *QueryServiceImpl) queryDataContext(ctx context.Context, account, path string, qry url.Values) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.xixunyun.com/"+path+"?"+qry.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("无法创建签到查询，请稍后重试")
	}

	req.Header.Set("User-Agent", "okhttp/3.8.0")

	resp, err := doUpstreamRequest(q.userRepo, q.client, req, account)
	if err != nil {
		if errors.Is(err, service.ErrUpstreamLoginRequired) {
			return nil, err
		}
		return nil, fmt.Errorf("无法连接习讯云，暂时无法核实签到记录，请稍后刷新")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("习讯云查询服务暂不可用，请稍后刷新")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("签到记录读取中断，请重新刷新核对")
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("习讯云返回的签到记录无法读取，请稍后刷新")
	}

	code, ok := result["code"].(float64)
	if !ok || code != 20000 {
		return nil, fmt.Errorf("习讯云未能完成查询，请稍后重试；若持续失败，请重新连接账号")
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("习讯云未返回有效的签到数据，请稍后刷新")
	}

	return data, nil
}

// SearchSchool searches for schools by name.
func (q *QueryServiceImpl) SearchSchool(schoolName string) ([]service.SchoolInfo, error) {
	empty, err := utils.IsSchoolInfoTableEmpty()
	if err != nil {
		return nil, fmt.Errorf("检查学校列表失败: %w", err)
	}
	if empty {
		if err := utils.FetchAndSaveSchoolData(); err != nil {
			return nil, fmt.Errorf("获取学校列表失败: %w", err)
		}
	}
	// TODO: Ideally, SearchSchoolID in utils should return []service.SchoolInfo
	// or this logic should be moved entirely into the repository.
	// For now, we call the utils function and convert the type.
	utilsSchools, err := utils.SearchSchoolID(schoolName)
	if err != nil {
		return nil, fmt.Errorf("数据库查询失败: %w", err)
	}
	if len(utilsSchools) == 0 {
		return nil, nil // Or return an empty slice: []service.SchoolInfo{}
	}

	// Convert []utils.SchoolInfo to []service.SchoolInfo
	serviceSchools := make([]service.SchoolInfo, len(utilsSchools))
	for i, uSchool := range utilsSchools {
		serviceSchools[i] = service.SchoolInfo{ // Assuming fields match
			SchoolID:   uSchool.SchoolID,
			SchoolName: uSchool.SchoolName,
		}
	}

	return serviceSchools, nil
}
