package impl

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"xixunyunsign/service"
)

type reportRepo struct {
	service.UserRepository
	token string
}

func (r reportRepo) GetUser(string) (string, string, string, error) { return r.token, "", "", nil }

type reportTransport func(*http.Request) (*http.Response, error)

func (f reportTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func reportResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func testReports(t *testing.T, fn reportTransport) *ReportServiceImpl {
	t.Helper()
	r := NewReportService(reportRepo{token: "test-token"}).(*ReportServiceImpl)
	r.client = &http.Client{Transport: fn}
	return r
}

func TestReportsListProtocol(t *testing.T) {
	for _, kind := range []string{"week", "month"} {
		t.Run(kind, func(t *testing.T) {
			r := testReports(t, func(req *http.Request) (*http.Response, error) {
				q := req.URL.Query()
				if req.URL.Path != "/Reports/StudentSearch" || q.Get("business_type") != kind || q.Get("token") != "test-token" || q.Get("page_no") != "2" || q.Get("mark_state") != "approval" {
					t.Fatalf("incorrect list protocol: path=%s type=%s", req.URL.Path, q.Get("business_type"))
				}
				return reportResponse(`{"code":20000,"data":{"list":[{"id":"1"}],"count":31}}`), nil
			})
			data, err := r.List(context.Background(), "user", service.ReportFilter{BusinessType: kind, Page: 2, PageSize: 15, MarkState: "approval"})
			if err != nil || data["count"] != float64(31) {
				t.Fatalf("list=%v error=%v", data, err)
			}
		})
	}
}

func TestReportsSubmitUsesSchoolTemplate(t *testing.T) {
	for _, kind := range []string{"week", "month"} {
		t.Run(kind, func(t *testing.T) {
			posted := false
			r := testReports(t, func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/Reports/Setting" {
					return reportResponse(`{"code":20000,"data":[{"title":"工作情况","require":1},{"title":"收获","require":"0"}]}`), nil
				}
				if req.URL.Path != "/Reports/StudentOperator" || req.Method != "POST" {
					t.Fatalf("unexpected endpoint %s", req.URL.Path)
				}
				req.ParseForm()
				if req.Form.Get("business_type") != kind || req.Form.Get("start_date") != "2026-10-01" || req.Form.Get("attachment") != "/upload/a.jpg" {
					t.Fatalf("wrong submission values")
				}
				var fields []service.ReportField
				if err := json.Unmarshal([]byte(req.Form.Get("content")), &fields); err != nil || fields[0].Require != "1" || fields[0].Content != "完成工作" || fields[1].Sort != 2 {
					t.Fatalf("incorrect template content: %v %v", fields, err)
				}
				posted = true
				return reportResponse(`{"code":20000,"message":"成功","data":{"id":"new"}}`), nil
			})
			_, err := r.Submit(context.Background(), "user", service.ReportSubmission{BusinessType: kind, StartDate: "2026-10-01", EndDate: "2026-10-07", Content: []service.ReportField{{Title: "工作情况", Content: "完成工作", Require: "0", Sort: 1}, {Title: "收获", Sort: 2}}, Attachments: []string{"/upload/a.jpg"}})
			if err != nil || !posted {
				t.Fatalf("posted=%v error=%v", posted, err)
			}
		})
	}
}

func TestReportsRejectInvalidSubmission(t *testing.T) {
	valid := service.ReportSubmission{BusinessType: "week", StartDate: "2026-10-01", EndDate: "2026-10-07", Content: []service.ReportField{{Title: "工作情况", Content: "正文", Sort: 1}}}
	for _, name := range []string{"type", "dates", "required", "template", "attachments"} {
		t.Run(name, func(t *testing.T) {
			input := valid
			input.Content = append([]service.ReportField(nil), valid.Content...)
			switch name {
			case "type":
				input.BusinessType = "other"
			case "dates":
				input.EndDate = "2026-09-30"
			case "required":
				input.Content[0].Content = " "
			case "template":
				input.Content[0].Title = "伪造模板"
			case "attachments":
				input.Attachments = []string{"a", "b", "c", "d"}
			}
			r := testReports(t, func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/Reports/Setting" {
					t.Fatal("invalid submission sent upstream")
				}
				return reportResponse(`{"code":20000,"data":[{"title":"工作情况","require":"1"}]}`), nil
			})
			_, err := r.Submit(context.Background(), "user", input)
			if !errors.Is(err, service.ErrReportInput) {
				t.Fatalf("wanted validation error, got %v", err)
			}
		})
	}
}

func TestReportsFailureAndMissingAccount(t *testing.T) {
	r := testReports(t, func(req *http.Request) (*http.Response, error) {
		return reportResponse(`{"code":40001,"message":"登录已过期"}`), nil
	})
	if _, err := r.List(context.Background(), "user", service.ReportFilter{BusinessType: "week"}); err == nil || !strings.Contains(err.Error(), "登录已过期") {
		t.Fatalf("lost upstream error: %v", err)
	}
	r.userRepo = reportRepo{}
	if _, err := r.Template(context.Background(), "user", "week"); !errors.Is(err, service.ErrReportAccount) {
		t.Fatalf("missing account error: %v", err)
	}
}

func TestReportsUploadProtocol(t *testing.T) {
	r := testReports(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/file/form" {
			t.Fatalf("wrong upload endpoint")
		}
		if err := req.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if req.FormValue("input_name") != "addFile" || req.FormValue("business") != "report" {
			t.Fatal("wrong upload fields")
		}
		f, h, err := req.FormFile("addFile[]")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		b, _ := io.ReadAll(f)
		if h.Filename != "note.txt" || string(b) != "attachment" {
			t.Fatal("attachment changed")
		}
		return reportResponse(`{"code":20000,"data":{"uri":"/upload/note.txt"}}`), nil
	})
	uri, err := r.Upload(context.Background(), "user", "note.txt", strings.NewReader("attachment"))
	if err != nil || uri != "/upload/note.txt" {
		t.Fatalf("upload=%s error=%v", uri, err)
	}
}

func TestReportsSubmitAcceptsScalarSuccess(t *testing.T) {
	r := testReports(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/Reports/Setting" {
			return reportResponse(`{"code":20000,"data":[{"title":"工作","require":"1"}]}`), nil
		}
		return reportResponse(`{"code":20000,"message":"成功","data":"42"}`), nil
	})
	_, err := r.Submit(context.Background(), "user", service.ReportSubmission{BusinessType: "month", StartDate: "2026-10-01", EndDate: "2026-10-31", Content: []service.ReportField{{Title: "工作", Content: "实际工作", Sort: 1}}})
	if err != nil {
		t.Fatalf("successful submission reported as failure: %v", err)
	}
}
