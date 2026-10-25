package impl

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"xixunyunsign/utils"
)

type schoolTransport func(*http.Request) (*http.Response, error)

func (transport schoolTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestSchoolSearchBootstrapsEmptyDatabase(t *testing.T) {
	previousDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		utils.CloseDB()
		_ = os.Chdir(previousDirectory)
	})
	previousTransport := http.DefaultTransport
	requestCount := 0
	http.DefaultTransport = schoolTransport(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://api.xixunyun.com/login/schoolmap" {
			t.Fatalf("unexpected school list URL: %s", request.URL)
		}
		requestCount++
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"code":20000,"message":"成功","data":[{"name":"杭州","id":"1","list":[{"school_id":"123","school_name":"测试大学"}]}]}`))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })
	query := NewQueryService(nil)
	for attempt := 0; attempt < 2; attempt++ {
		schools, err := query.SearchSchool("测试")
		if err != nil {
			t.Fatal(err)
		}
		if len(schools) != 1 || schools[0].SchoolID != "123" {
			t.Fatalf("school search should load the school list on first use, got %+v", schools)
		}
	}
	if requestCount != 1 {
		t.Fatalf("populated school list should be reused, requests=%d", requestCount)
	}
}
