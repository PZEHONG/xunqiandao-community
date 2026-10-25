package service

import (
	"context"
	"errors"
	"io"
)

var ErrReportInput = errors.New("报告参数错误")
var ErrReportAccount = errors.New("请先连接习讯云账号")

const MaxReportFileSize int64 = 10 << 20

type ReportField struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Require string `json:"require"`
	Sort    int    `json:"sort"`
}
type ReportFilter struct {
	BusinessType                  string
	Page, PageSize                int
	StartTime, EndTime, MarkState string
}
type ReportSubmission struct {
	BusinessType string        `json:"business_type"`
	StartDate    string        `json:"start_date"`
	EndDate      string        `json:"end_date"`
	Content      []ReportField `json:"content"`
	Attachments  []string      `json:"attachments"`
}
type ReportService interface {
	List(context.Context, string, ReportFilter) (map[string]interface{}, error)
	Template(context.Context, string, string) ([]ReportField, error)
	Detail(context.Context, string, string) (map[string]interface{}, error)
	Submit(context.Context, string, ReportSubmission) (map[string]interface{}, error)
	Upload(context.Context, string, string, io.Reader) (string, error)
}
