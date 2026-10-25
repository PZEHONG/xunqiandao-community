package impl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"xixunyunsign/service"
)

type ReportServiceImpl struct {
	userRepo service.UserRepository
	client   *http.Client
}

func NewReportService(repo service.UserRepository) service.ReportService {
	return &ReportServiceImpl{userRepo: repo, client: &http.Client{Timeout: 25 * time.Second}}
}
func reportInput(message string) error { return fmt.Errorf("%w：%s", service.ErrReportInput, message) }
func reportType(kind string) error {
	if kind != "week" && kind != "month" {
		return reportInput("请选择周报或月报")
	}
	return nil
}
func reportDates(start, end string, required bool) error {
	if !required && start == "" && end == "" {
		return nil
	}
	a, err := time.Parse("2006-01-02", start)
	if err != nil {
		return reportInput("请填写有效的开始日期")
	}
	b, err := time.Parse("2006-01-02", end)
	if err != nil || b.Before(a) {
		return reportInput("结束日期必须不早于开始日期")
	}
	return nil
}
func (r *ReportServiceImpl) token(account string) (string, error) {
	if strings.TrimSpace(account) == "" || r.userRepo == nil {
		return "", service.ErrReportAccount
	}
	token, _, _, err := r.userRepo.GetUser(account)
	if err != nil || token == "" {
		return "", service.ErrReportAccount
	}
	return token, nil
}

// The token stays on the server; transport errors never include its URL.
func (r *ReportServiceImpl) request(ctx context.Context, account, method, path string, params url.Values, body io.Reader, contentType string) (json.RawMessage, error) {
	token, err := r.token(account)
	if err != nil {
		return nil, err
	}
	if params == nil {
		params = url.Values{}
	}
	params.Set("token", token)
	req, err := http.NewRequestWithContext(ctx, method, "https://api.xixunyun.com/"+path+"?"+params.Encode(), body)
	if err != nil {
		return nil, fmt.Errorf("无法创建报告请求")
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("Origin", "https://www.xixunyun.com")
	req.Header.Set("Referer", "https://www.xixunyun.com/")
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := doUpstreamRequest(r.userRepo, r.client, req, account)
	if err != nil {
		if errors.Is(err, service.ErrUpstreamLoginRequired) {
			return nil, err
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("无法连接习讯云报告服务，请稍后重试")
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取报告响应失败")
	}
	var result struct {
		Code    json.Number     `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal(payload, &result) != nil {
		return nil, fmt.Errorf("习讯云返回了无法读取的报告结果")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || result.Code.String() != "20000" {
		message := strings.ReplaceAll(result.Message, token, "[已隐藏]")
		if message == "" {
			message = "报告操作失败，请稍后重试"
		}
		return nil, fmt.Errorf("%s", message)
	}
	return result.Data, nil
}
func reportObject(raw json.RawMessage, allowEmpty bool) (map[string]interface{}, error) {
	data := map[string]interface{}{}
	if allowEmpty && (len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" || string(raw) == `""`) {
		return data, nil
	}
	if json.Unmarshal(raw, &data) != nil || data == nil {
		if allowEmpty {
			return map[string]interface{}{}, nil
		}
		return nil, fmt.Errorf("报告数据格式错误")
	}
	stripReportSecrets(data)
	return data, nil
}
func stripReportSecrets(data interface{}) {
	switch value := data.(type) {
	case map[string]interface{}:
		for key, item := range value {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "secret") {
				delete(value, key)
			} else {
				stripReportSecrets(item)
			}
		}
	case []interface{}:
		for _, item := range value {
			stripReportSecrets(item)
		}
	}
}
func (r *ReportServiceImpl) List(ctx context.Context, account string, filter service.ReportFilter) (map[string]interface{}, error) {
	if err := reportType(filter.BusinessType); err != nil {
		return nil, err
	}
	if err := reportDates(filter.StartTime, filter.EndTime, false); err != nil {
		return nil, err
	}
	if filter.Page == 0 {
		filter.Page = 1
	}
	if filter.PageSize == 0 {
		filter.PageSize = 15
	}
	if filter.Page < 1 || filter.PageSize < 1 || filter.PageSize > 100 {
		return nil, reportInput("分页参数无效")
	}
	if filter.MarkState != "" && filter.MarkState != "approval" && filter.MarkState != "without_approval" {
		return nil, reportInput("批阅状态无效")
	}
	raw, err := r.request(ctx, account, http.MethodGet, "Reports/StudentSearch", url.Values{"business_type": {filter.BusinessType}, "page_no": {strconv.Itoa(filter.Page)}, "page_size": {strconv.Itoa(filter.PageSize)}, "start_time": {filter.StartTime}, "end_time": {filter.EndTime}, "mark_state": {filter.MarkState}, "order": {"create_time.desc"}}, nil, "")
	if err != nil {
		return nil, err
	}
	return reportObject(raw, false)
}
func (r *ReportServiceImpl) Template(ctx context.Context, account, kind string) ([]service.ReportField, error) {
	if err := reportType(kind); err != nil {
		return nil, err
	}
	raw, err := r.request(ctx, account, http.MethodGet, "Reports/Setting", url.Values{"business_type": {kind}}, nil, "")
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Title   string          `json:"title"`
		Require json.RawMessage `json:"require"`
	}
	if json.Unmarshal(raw, &rows) != nil || len(rows) == 0 {
		return nil, fmt.Errorf("学校尚未配置此报告模板，请在习讯云确认")
	}
	fields := make([]service.ReportField, 0, len(rows))
	for i, row := range rows {
		if strings.TrimSpace(row.Title) == "" {
			return nil, fmt.Errorf("学校报告模板缺少标题")
		}
		required := "0"
		switch string(row.Require) {
		case "1", `"1"`, "true":
			required = "1"
		}
		fields = append(fields, service.ReportField{Title: row.Title, Require: required, Sort: i + 1})
	}
	return fields, nil
}
func (r *ReportServiceImpl) Detail(ctx context.Context, account, id string) (map[string]interface{}, error) {
	if strings.TrimSpace(id) == "" {
		return nil, reportInput("缺少报告编号")
	}
	raw, err := r.request(ctx, account, http.MethodGet, "Reports/Detail", url.Values{"id": {id}}, nil, "")
	if err != nil {
		return nil, err
	}
	return reportObject(raw, false)
}
func (r *ReportServiceImpl) Submit(ctx context.Context, account string, input service.ReportSubmission) (map[string]interface{}, error) {
	if err := reportType(input.BusinessType); err != nil {
		return nil, err
	}
	if err := reportDates(input.StartDate, input.EndDate, true); err != nil {
		return nil, err
	}
	if len(input.Attachments) > 3 {
		return nil, reportInput("最多上传三个附件")
	}
	for _, uri := range input.Attachments {
		if !validReportAttachment(uri) {
			return nil, reportInput("附件地址无效，请重新上传")
		}
	}
	fields, err := r.Template(ctx, account, input.BusinessType)
	if err != nil {
		return nil, err
	}
	if len(fields) != len(input.Content) {
		return nil, reportInput("学校模板已变更，请重新打开填写页面")
	}
	total := 0
	for i := range fields {
		if fields[i].Title != input.Content[i].Title || fields[i].Sort != input.Content[i].Sort {
			return nil, reportInput("学校模板已变更，请重新打开填写页面")
		}
		fields[i].Content = strings.TrimSpace(input.Content[i].Content)
		total += len(fields[i].Content)
		if fields[i].Require == "1" && fields[i].Content == "" {
			return nil, reportInput(fields[i].Title + "为必填项")
		}
	}
	if total > 200000 {
		return nil, reportInput("报告内容过长")
	}
	content, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	form := url.Values{"business_type": {input.BusinessType}, "start_date": {input.StartDate}, "end_date": {input.EndDate}, "content": {string(content)}, "attachment": {strings.Join(input.Attachments, ",")}}
	raw, err := r.request(ctx, account, http.MethodPost, "Reports/StudentOperator", nil, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded; charset=UTF-8")
	if err != nil {
		return nil, err
	}
	return reportObject(raw, true)
}
func validReportAttachment(uri string) bool {
	if strings.TrimSpace(uri) == "" || strings.ContainsAny(uri, ",\r\n\x00") {
		return false
	}
	u, err := url.Parse(uri)
	return err == nil && (u.Scheme == "" || u.Scheme == "http" || u.Scheme == "https")
}
func (r *ReportServiceImpl) Upload(ctx context.Context, account, filename string, file io.Reader) (string, error) {
	if _, err := r.token(account); err != nil {
		return "", err
	}
	data, err := io.ReadAll(io.LimitReader(file, service.MaxReportFileSize+1))
	if err != nil {
		return "", fmt.Errorf("读取附件失败")
	}
	if len(data) == 0 || int64(len(data)) > service.MaxReportFileSize {
		return "", reportInput("单个附件需为 1 字节至 10 MB")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("addFile[]", filepath.Base(filename))
	if err != nil {
		return "", reportInput("附件名称无效")
	}
	if _, err = part.Write(data); err != nil {
		return "", err
	}
	for key, value := range map[string]string{"input_name": "addFile", "business": "report"} {
		if err = writer.WriteField(key, value); err != nil {
			return "", err
		}
	}
	if err = writer.Close(); err != nil {
		return "", err
	}
	raw, err := r.request(ctx, account, http.MethodPost, "file/form", nil, &body, writer.FormDataContentType())
	if err != nil {
		return "", err
	}
	var result struct {
		URI string `json:"uri"`
	}
	if json.Unmarshal(raw, &result) != nil || !validReportAttachment(result.URI) {
		return "", fmt.Errorf("附件上传未返回有效地址，请重试")
	}
	return result.URI, nil
}
