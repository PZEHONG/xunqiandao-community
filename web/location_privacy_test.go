package web

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
	"xixunyunsign/service"
)

type privacySignStub struct {
	service.SignService
	calls   int
	args    []string
	options service.SignOptions
}

func (s *privacySignStub) SignInWithOptions(_ context.Context, account, address, name, lat, lon string, options service.SignOptions) (string, error) {
	s.options = options
	return s.SignIn(account, address, name, lat, lon)
}

func (s *privacySignStub) SignIn(account, address, name, lat, lon string) (string, error) {
	s.calls++
	s.args = []string{account, address, name, lat, lon}
	return "签到成功", nil
}

func TestSignRequiresExplicitConfirmationAndCompleteCoordinates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{
		`{"account":"u","address":"广东省广州市参考地点","latitude":"23","longitude":"113"}`,
		`{"account":"u","address":"广东省广州市参考地点","location_confirmed":true}`,
		`{"account":"u","address":"广东省广州市参考地点","location_confirmed":true,"latitude":"NaN","longitude":"113"}`,
		`{"account":"u","address":"广东省广州市参考地点","location_confirmed":true,"latitude":"23","longitude":"181"}`,
		`{"account":"u","address":"广东省广州市参考地点","location_confirmed":true,"latitude":"23.123456","longitude":"113.654321"}`,
	} {
		s := &privacySignStub{}
		r := authenticatedRouter(t, &Handlers{SignService: s})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", "/api/sign", strings.NewReader(body)))
		if w.Code != 400 || s.calls != 0 {
			t.Fatalf("unconfirmed/invalid location submitted: status=%d calls=%d", w.Code, s.calls)
		}
	}
	s := &privacySignStub{}
	w := httptest.NewRecorder()
	place := signLocation(locationResult{Address: "广东省广州市参考地点", Name: "参考地点", Latitude: "23.123456", Longitude: "113.654321"})
	req := SignRequest{Account: "u", Address: place.Address, AddressName: place.Name, Latitude: place.Latitude, Longitude: place.Longitude, LocationConfirmed: true, ConfirmationToken: place.ConfirmationToken}
	body, _ := json.Marshal(req)
	authenticatedRouter(t, &Handlers{SignService: s}).ServeHTTP(w, httptest.NewRequest("POST", "/api/sign", bytes.NewReader(body)))
	if w.Code != 200 || s.calls != 1 || strings.Join(s.args, "|") != "u|广东省广州市参考地点|参考地点|23.123456|113.654321" {
		t.Fatalf("confirmed snapshot changed: status=%d args=%v", w.Code, s.args)
	}
	for _, field := range []string{"address", "name", "latitude", "longitude", "token", "expired"} {
		t.Run(field, func(t *testing.T) {
			modified := req
			switch field {
			case "address":
				modified.Address += "变更"
			case "name":
				modified.AddressName += "变更"
			case "latitude":
				modified.Latitude = "24.123456"
			case "longitude":
				modified.Longitude = "114.654321"
			case "token":
				modified.ConfirmationToken = "forged"
			case "expired":
				modified.ConfirmationToken = "1." + strings.Split(req.ConfirmationToken, ".")[1]
			}
			body, _ := json.Marshal(modified)
			w := httptest.NewRecorder()
			authenticatedRouter(t, &Handlers{SignService: s}).ServeHTTP(w, httptest.NewRequest("POST", "/api/sign", bytes.NewReader(body)))
			if w.Code != 400 || s.calls != 1 {
				t.Fatalf("altered candidate accepted: status=%d calls=%d", w.Code, s.calls)
			}
		})
	}
}

func TestLocationPrivacyHeadersAndNoRequestLogging(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var log bytes.Buffer
	oldOut, oldErr := gin.DefaultWriter, gin.DefaultErrorWriter
	gin.DefaultWriter, gin.DefaultErrorWriter = &log, &log
	defer func() { gin.DefaultWriter, gin.DefaultErrorWriter = oldOut, oldErr }()
	r := authenticatedRouter(t, &Handlers{})
	r.GET("/privacy-panic", func(*gin.Context) { panic("private-address-sentinel") })
	for _, path := range []string{"/", "/assets/locations.js", "/api/health?location=private-address-sentinel", "/privacy-panic?latitude=23.123456&longitude=113.654321"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Header().Get("Permissions-Policy") != "geolocation=()" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("missing privacy headers on %s", path)
		}
	}
	if log.Len() != 0 {
		t.Fatalf("request or panic details were logged: %s", log.String())
	}
	for _, path := range []string{"/api/locations/search?keyword=reference", "/api/locations/reverse?longitude=113&latitude=23"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatalf("location GET endpoint still enabled: %s", path)
		}
	}
}
