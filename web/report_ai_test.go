package web

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"xixunyunsign/service"
)

func TestAIConfigReadsDotEnvAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	t.Setenv("REPORT_AI_CONFIG_FILE", path)
	t.Setenv("REPORT_AI_API_KEY", "")
	t.Setenv("REPORT_AI_BASE_URL", "")
	t.Setenv("REPORT_AI_MODEL", "")
	// Unset in this test so file settings apply; t.Setenv restores the original state.
	for _, key := range []string{"REPORT_AI_API_KEY", "REPORT_AI_BASE_URL", "REPORT_AI_MODEL"} {
		os.Unsetenv(key)
	}
	if err := os.WriteFile(path, []byte("\ufeffREPORT_AI_API_KEY='file-secret' # local\nREPORT_AI_MODEL=test-model\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, model, key, err := loadReportAIConfig()
	if err != nil || key != "file-secret" || model != "test-model" {
		t.Fatal("file configuration not loaded")
	}
	os.WriteFile(path, []byte("REPORT_AI_API_KEY=updated-secret\n"), 0600)
	_, _, key, err = loadReportAIConfig()
	if err != nil || key != "updated-secret" {
		t.Fatal("file update not reloaded")
	}
	t.Setenv("REPORT_AI_API_KEY", "process-secret")
	_, _, key, err = loadReportAIConfig()
	if err != nil || key != "process-secret" {
		t.Fatal("process override lost")
	}
}

func TestAISuggestDates(t *testing.T) {
	now := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name, kind, mode string
		history          []aiPeriod
		start, end       string
	}{
		{"monthly gap", "month", "batch", []aiPeriod{{Start: "2026-07-01", End: "2026-07-31"}, {Start: "2026-09-01", End: "2026-09-30"}}, "2026-08-01", "2026-10-29"},
		{"monthly next", "month", "single", []aiPeriod{{Start: "2026-09-01", End: "2026-09-30"}}, "2026-10-01", "2026-10-31"},
		{"weekly last submission", "week", "single", []aiPeriod{{Start: "2026-10-01", End: "2026-10-07"}, {Start: "2026-10-15", End: "2026-10-21"}}, "2026-10-07", "2026-10-14"},
		{"weekly next batch", "week", "batch", []aiPeriod{{Start: "2026-10-01", End: "2026-10-07"}, {Start: "2026-10-08", End: "2026-10-14"}}, "2026-10-07", "2026-10-29"},
		{"no history month", "month", "batch", nil, "2026-10-01", "2026-10-29"},
		{"no history week", "week", "single", nil, "2026-10-25", "2026-11-01"},
		{"up to date batch", "month", "batch", []aiPeriod{{Start: "2026-10-01", End: "2026-10-31"}}, "2026-10-01", "2026-10-29"},
		{"up to date next single", "month", "single", []aiPeriod{{Start: "2026-10-01", End: "2026-10-31"}}, "2026-11-01", "2026-11-30"},
		{"future periods already covered", "month", "single", []aiPeriod{{Start: "2026-10-01", End: "2026-12-31"}}, "2027-01-01", "2027-01-31"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := suggestAIDates(tt.kind, tt.mode, tt.history, now)
			if got.Start != tt.start || got.End != tt.end {
				t.Fatalf("got %+v expected %s %s", got, tt.start, tt.end)
			}
		})
	}
}

func TestAISuggestOldMonthlyHistoryStaysWithinPlannerLimit(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	got := suggestAIDates("month", "batch", []aiPeriod{{Start: "2020-09-01", End: "2020-09-30"}}, now)
	if _, err := aiPeriods("month", got.Start, got.End); err != nil {
		t.Fatalf("auto suggested invalid range %+v: %v", got, err)
	}
}

func TestAIPeriodsAndCoverage(t *testing.T) {
	weeks, err := aiPeriods("week", "2026-09-29", "2026-10-14")
	if err != nil || len(weeks) != 3 || weeks[0].End != "2026-10-06" || weeks[2].End != "2026-10-14" {
		t.Fatalf("anchored weeks: %+v %v", weeks, err)
	}
	months, err := aiPeriods("month", "2024-02-20", "2024-03-02")
	if err != nil || len(months) != 2 || months[0].End != "2024-02-29" {
		t.Fatalf("months: %+v %v", months, err)
	}
	if _, err := aiPeriods("week", "2026-02-30", "2026-03-03"); err == nil {
		t.Fatal("invalid date accepted")
	}
	classified, err := planAIReportDates("week", "2026-09-29", "2026-10-14", []aiPeriod{{Start: "2026-09-29", End: "2026-10-05"}, {Start: "2026-10-07", End: "2026-10-12"}})
	if err != nil || len(classified) != 3 || classified[0].State != "missing" || classified[1].Start != "2026-10-06" || classified[1].End != "2026-10-13" || classified[1].State != "missing" || classified[2].State != "missing" {
		t.Fatalf("coverage: %+v", classified)
	}
	if got := reportRecordDate(float64(1790784000)); got != "2026-10-01" {
		t.Fatalf("Beijing date = %s", got)
	}
}

type aiHistoryStub struct {
	reportStub
	bad      bool
	calls    int
	details  int
	changing bool
}

func (s *aiHistoryStub) List(_ context.Context, _ string, f service.ReportFilter) (map[string]interface{}, error) {
	s.calls++
	if f.StartTime != "" || f.EndTime != "" || f.MarkState != "" {
		return nil, fmt.Errorf("must scan full history")
	}
	row := map[string]interface{}{"id": "old", "create_time": "2026-10-29"}
	if f.Page == 2 && s.changing {
		return map[string]interface{}{"count": 1, "list": []interface{}{}}, nil
	}
	if f.Page == 2 {
		row = map[string]interface{}{"id": "new", "start_date": "2026-10-08", "end_date": "2026-10-15"}
	}
	return map[string]interface{}{"count": 2, "list": []interface{}{row}}, nil
}

func TestAIMissingRejectsChangingHistoryCount(t *testing.T) {
	s := &aiHistoryStub{changing: true}
	if periods, err := missingAIReports(context.Background(), s, "test", "week", "2026-10-01", "2026-10-21"); err == nil {
		t.Fatalf("accepted incomplete scan: %+v", periods)
	}
}
func (s *aiHistoryStub) Detail(context.Context, string, string) (map[string]interface{}, error) {
	s.details++
	if s.bad {
		return map[string]interface{}{"create_time": "2026-10-29"}, nil
	}
	return map[string]interface{}{"start_date": "2026-10-01", "end_date": "2026-10-08"}, nil
}
func TestAIMissingScansPagesAndDetailNotSubmissionDate(t *testing.T) {
	s := &aiHistoryStub{}
	periods, err := missingAIReports(context.Background(), s, "test", "week", "2026-10-01", "2026-10-21")
	if err != nil || len(periods) != 3 || periods[0].State != "existing" || periods[1].State != "existing" || periods[2].State != "missing" || s.calls != 2 || s.details != 1 {
		t.Fatalf("periods=%+v error=%v calls=%d detail=%d", periods, err, s.calls, s.details)
	}
	s.bad = true
	if _, err := missingAIReports(context.Background(), s, "test", "week", "2026-10-01", "2026-10-21"); err == nil {
		t.Fatal("unknown report dates classified as missing")
	}
}

func TestAIRoutesNeverSubmitAndHideKey(t *testing.T) {
	t.Setenv("REPORT_AI_API_KEY", "server-fixture-secret")
	t.Setenv("REPORT_AI_BASE_URL", "https://ai.example.invalid/v1")
	t.Setenv("REPORT_AI_MODEL", "test-model")
	s := &reportStub{}
	router := authenticatedRouter(t, &Handlers{ReportService: s})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/reports/ai/config?account=test", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), "server-fixture-secret") || !strings.Contains(w.Body.String(), `"configured":true`) {
		t.Fatalf("config %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/reports/ai/missing?account=test&business_type=week&start=2026-10-01&end=2026-10-14", nil))
	if w.Code != 200 || strings.Count(w.Body.String(), `"state":"missing"`) != 2 || s.last.BusinessType != "" {
		t.Fatalf("missing %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/reports/ai/dates?account=test&business_type=month&mode=batch", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"start":`) || !strings.Contains(w.Body.String(), `"end":`) || s.last.BusinessType != "" {
		t.Fatalf("dates %d %s", w.Code, w.Body.String())
	}
	t.Setenv("REPORT_AI_API_KEY", "")
	w = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/reports/ai/generate", strings.NewReader(`{"account":"test","business_type":"week","start":"2026-10-01","end":"2026-10-07","notes":"实习素材"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != 503 || !strings.Contains(w.Body.String(), "密钥") || s.last.BusinessType != "" {
		t.Fatalf("generate %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/reports/ai/generate", strings.NewReader(`{"account":"test","business_type":"week","start":"2026-10-01","end":"2026-10-07","notes":"素材","api_key":"browser-key-must-be-ignored"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatalf("client key incorrectly accepted: %d", w.Code)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/reports/ai/config?account=someone-else", nil))
	if w.Code != 403 {
		t.Fatalf("ownership: %d", w.Code)
	}
}

func TestAIChatCompletionsStream(t *testing.T) {
	for _, tt := range []struct {
		name, prefix, tail, want string
		ok                       bool
	}{
		{"complete", "", "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", "正文", true},
		{"terminal without trailing newline", "", "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]", "正文", true},
		{"truncated", "", "", "正文", false},
		{"done without finish reason", "", "data: [DONE]\n\n", "正文", false},
		{"stop without done", "", "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n", "正文", false},
		{"length limit", "", "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n", "正文", false},
		{"filtered", "", "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"content_filter\"}]}\n\ndata: [DONE]\n\n", "正文", false},
		{"tool call is not report text", "", "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n", "正文", false},
		{"provider error hidden", "", "data: {\"error\":{\"message\":\"secret\"}}\n\n", "正文", false},
		{"malformed", "", "data: not-json\n\n", "正文", false},
		{"role reasoning and other choices ignored", "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"reasoning_content\":\"secret\"},\"finish_reason\":null},{\"index\":1,\"delta\":{\"content\":\"secret\"},\"finish_reason\":null}]}\n\n", "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"completion_tokens\":2}}\n\ndata: [DONE]\n\n", "正文", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fixture-key" || r.URL.Path != "/chat/completions" || r.Method != "POST" {
					t.Error("provider request")
				}
				var body struct {
					Model     string                           `json:"model"`
					Stream    bool                             `json:"stream"`
					Store     *bool                            `json:"store"`
					MaxTokens int                              `json:"max_tokens"`
					Messages  []struct{ Role, Content string } `json:"messages"`
				}
				if json.NewDecoder(r.Body).Decode(&body) != nil || body.Model != "smart-auto" || !body.Stream || body.Store == nil || *body.Store || body.MaxTokens <= 0 || len(body.Messages) != 2 || body.Messages[0].Role != "system" || !strings.Contains(body.Messages[0].Content, "不得编造") || body.Messages[1].Role != "user" || body.Messages[1].Content != "素材" {
					t.Error("expected streaming Chat Completions messages with report instructions and material")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, tt.prefix+": heartbeat\r\ndata: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"正文\"},\"finish_reason\":null}]}\r\n\r\n"+tt.tail)
			}))
			defer server.Close()
			text := ""
			err := streamAIResponse(context.Background(), server.Client(), server.URL, "fixture-key", "smart-auto", "素材", func(delta string) error { text += delta; return nil })
			if (err == nil) != tt.ok || text != tt.want {
				t.Fatalf("stream=%q err=%v", text, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("provider error leaked")
			}
		})
	}
}

func TestAIChatStreamsBeforeCompletion(t *testing.T) {
	seen := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"实时正文\"},\"finish_reason\":null}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-seen:
			io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	err := streamAIResponse(ctx, server.Client(), server.URL, "key", "model", "素材", func(delta string) error {
		if delta != "实时正文" {
			t.Errorf("unexpected text: %q", delta)
		}
		close(seen)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAIChatRejectsEmptyAndNonStreamingResponses(t *testing.T) {
	for _, tt := range []struct{ name, contentType, body string }{
		{"empty", "text/event-stream", "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"},
		{"json response", "application/json", `{\"choices\":[{\"message\":{\"content\":\"正文\"}}]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				io.WriteString(w, tt.body)
			}))
			defer server.Close()
			if err := streamAIResponse(context.Background(), server.Client(), server.URL, "key", "model", "素材", func(string) error { t.Error("unexpected text"); return nil }); err == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
}

func TestAIChatStopsWhenConsumerCancels(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"正文\"}}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	stopped := errors.New("consumer stopped")
	err := streamAIResponse(ctx, server.Client(), server.URL, "key", "model", "素材", func(string) error { cancel(); return stopped })
	if !errors.Is(err, stopped) {
		t.Fatalf("cancel error = %v", err)
	}
}

func TestAIGenerateForwardsChatStreamBeforeCompletion(t *testing.T) {
	seen := make(chan struct{})
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Error("wrong provider endpoint")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"实时正文\"},\"finish_reason\":null}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-seen:
			io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		case <-r.Context().Done():
		}
	}))
	defer provider.Close()
	previous := http.DefaultTransport
	http.DefaultTransport = provider.Client().Transport
	defer func() { http.DefaultTransport = previous }()
	t.Setenv("REPORT_AI_BASE_URL", provider.URL)
	t.Setenv("REPORT_AI_API_KEY", "fixture-key")
	t.Setenv("REPORT_AI_MODEL", "test-model")
	s := &reportStub{}
	server := httptest.NewServer(authenticatedRouter(t, &Handlers{ReportService: s}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/api/reports/ai/generate", strings.NewReader(`{"account":"test","business_type":"week","start":"2026-10-01","end":"2026-10-07","notes":"测试素材"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatal("expected SSE response")
	}
	scanner := bufio.NewScanner(resp.Body)
	types := []string{}
	for scanner.Scan() {
		if !strings.HasPrefix(scanner.Text(), "data: ") {
			continue
		}
		var event struct {
			Type, Delta string
			Fields      []service.ReportField
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(scanner.Text(), "data: ")), &event); err != nil {
			t.Fatal(err)
		}
		types = append(types, event.Type)
		if event.Type == "delta" {
			if event.Delta != "实时正文" {
				t.Fatalf("delta=%q", event.Delta)
			}
			close(seen) // Provider cannot finish until its first delta reaches the client.
		}
		if event.Type == "done" && (len(event.Fields) != 1 || event.Fields[0].Content != "实时正文") {
			t.Fatal("incomplete final fields")
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(types, ",") != "field,delta,done" || s.last.BusinessType != "" {
		t.Fatalf("events=%v submission=%+v", types, s.last)
	}
}
