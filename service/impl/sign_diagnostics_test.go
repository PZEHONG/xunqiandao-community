package impl

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestSignDiagnosticLogsBusinessCodeWithoutCredentials(t *testing.T) {
	var out bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&out)
	defer log.SetOutput(previous)
	s := signFixture(func(*http.Request) (*http.Response, error) {
		return reportResponse(`{"code":40001,"message":"必须上传照片 private-token student-account"}`), nil
	})
	_, err := s.request(context.Background(), "student-account", "POST", "signin_rsa", nil, "")
	if err == nil {
		t.Fatal("expected upstream rejection")
	}
	text := out.String()
	for _, want := range []string{"[sign_upstream]", "endpoint=signin_rsa", "http_status=200", "code=40001", "必须上传照片"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	for _, secret := range []string{"private-token", "student-account", "?token="} {
		if strings.Contains(text, secret) {
			t.Fatal("credentials leaked")
		}
	}
}
func TestSignDiagnosticLogsHideRequestLocationAndTransportURL(t *testing.T) {
	for _, mode := range []string{"rejected", "transport", "invalid_json"} {
		t.Run(mode, func(t *testing.T) {
			var out bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&out)
			defer log.SetOutput(previous)
			s := signFixture(func(req *http.Request) (*http.Response, error) {
				if mode == "transport" {
					return nil, errors.New(req.URL.String())
				}
				if mode == "invalid_json" {
					return reportResponse("not-json"), nil
				}
				return reportResponse(`{"code":40002,"message":"必须上传照片 private-token student-account 广东省广州市隐私地址 23.123456 https://example.com/?token=private-token"}`), nil
			})
			_, err := s.request(context.Background(), "student-account", "POST", "signin_rsa", strings.NewReader("address="+url.QueryEscape("广东省广州市隐私地址")+"&latitude=23.123456"), "application/x-www-form-urlencoded")
			if err == nil {
				t.Fatal("expected failure")
			}
			text := out.String()
			if !strings.Contains(text, "[sign_upstream]") {
				t.Fatal("missing upstream log")
			}
			for _, secret := range []string{"private-token", "student-account", "广东省广州市隐私地址", "23.123456", "https://"} {
				if strings.Contains(text, secret) {
					t.Fatalf("secret leaked: %q", text)
				}
			}
		})
	}
}

func TestSignFailurePreservesSafeBusinessCode(t *testing.T) {
	s := signFixture(func(*http.Request) (*http.Response, error) {
		return reportResponse("{\"code\":40001,\"message\":\"必须上传照片 private-token student-account\"}"), nil
	})
	_, err := s.request(context.Background(), "student-account", "POST", "signin_rsa", nil, "")
	if err == nil || !strings.Contains(err.Error(), "40001") || strings.Contains(err.Error(), "private-token") || strings.Contains(err.Error(), "student-account") {
		t.Fatalf("missing safe failure detail: %v", err)
	}
}
