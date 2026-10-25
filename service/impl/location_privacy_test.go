package impl

import (
	"net/http"
	"testing"
	"time"
	"xixunyunsign/service"
)

type privacyRepo struct {
	service.UserRepository
	reads   int
	updates int
}

func TestQueryUsesCurrentMonth(t *testing.T) {
	q := NewQueryService(&privacyRepo{}).(*QueryServiceImpl)
	q.additionalUserData = func(string) (map[string]string, error) { return map[string]string{}, nil }
	want := time.Now().In(time.FixedZone("Shanghai", 8*60*60)).Format("2006-01")
	q.client = &http.Client{Transport: reportTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("month_date") != want {
			t.Errorf("query is stuck in a historical month: %q", req.URL.Query().Get("month_date"))
		}
		if req.URL.Path == "/signIn40/SignInList" {
			return reportResponse(`{"code":20000,"data":{"list":[]}}`), nil
		}
		return reportResponse(`{"code":20000,"data":{"sign_days":2}}`), nil
	})}
	_, err := q.QuerySignInfo("u")
	if err != nil {
		t.Fatal(err)
	}
}

func (r *privacyRepo) GetUser(string) (string, string, string, error) {
	r.reads++
	return "test-token", "23", "113", nil
}
func (r *privacyRepo) UpdateCoordinates(string, string, string) error { r.updates++; return nil }

func TestSignNeverFallsBackToStoredCoordinates(t *testing.T) {
	for _, pair := range [][2]string{{"", ""}, {"23", ""}, {"", "113"}, {"NaN", "113"}, {"91", "113"}, {"23", "Inf"}} {
		r := &privacyRepo{}
		_, err := NewSignService(r).SignIn("u", "广东省广州市参考地点", "", pair[0], pair[1])
		if err == nil || r.reads != 0 {
			t.Fatalf("missing/invalid explicit coordinates reached account lookup: %v, reads=%d", err, r.reads)
		}
	}
}

func TestQueryDoesNotPersistPlatformCoordinates(t *testing.T) {
	r := &privacyRepo{}
	q := NewQueryService(r).(*QueryServiceImpl)
	q.additionalUserData = func(string) (map[string]string, error) { return map[string]string{}, nil }
	q.client = &http.Client{Transport: reportTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/signIn40/SignInList" {
			return reportResponse(`{"code":20000,"data":{"list":[]}}`), nil
		}
		return reportResponse(`{"code":20000,"data":{"sign_resources_info":{"mid_sign_latitude":"23.123456","mid_sign_longitude":"113.654321"},"sign_days":2}}`), nil
	})}
	data, err := q.QuerySignInfo("u")
	if err != nil || data["sign_days"] != float64(2) || r.updates != 0 {
		t.Fatalf("query recorded coordinates: error=%v updates=%d", err, r.updates)
	}
	if _, ok := data["sign_resources_info"]; ok {
		t.Fatal("platform location metadata retained in query response")
	}
}
