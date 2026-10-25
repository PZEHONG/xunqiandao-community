package impl

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"xixunyunsign/service"
)

func TestSignRemarksAcceptsGzippedResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Encoding") != "gzip" {
			t.Error("automatic compression not enabled")
		}
		w.Header().Set("Content-Encoding", "gzip")
		out := gzip.NewWriter(w)
		defer out.Close()
		io.WriteString(out, `{"code":20000,"data":{"mark_list":[{"key":0,"value":"上班"},{"key":"42","value":"院区轮转"}]}}`)
	}))
	defer server.Close()
	s := signFixture(func(req *http.Request) (*http.Response, error) {
		copy := req.Clone(req.Context())
		copy.URL.Scheme = "http"
		copy.URL.Host = strings.TrimPrefix(server.URL, "http://")
		return http.DefaultTransport.RoundTrip(copy)
	})
	marks, err := s.Remarks(context.Background(), "u")
	if err != nil || len(marks) != 2 || marks[0].Key != "0" || marks[1].Key != "42" {
		t.Fatalf("gzip response unreadable: error=%v remarks=%v", err, marks)
	}
}

func signFixture(fn reportTransport) *SignServiceImpl {
	s := NewSignService(reportRepo{token: "private-token"}).(*SignServiceImpl)
	s.additionalUserData = func(string) (map[string]string, error) {
		return map[string]string{"school_id": "600", "entrance_year": "2024", "graduation_year": "2027"}, nil
	}
	s.client = &http.Client{Transport: fn}
	return s
}

func TestSignUsesSavedSchoolAndAppFields(t *testing.T) {
	posted := false
	s := signFixture(func(req *http.Request) (*http.Response, error) {
		q := req.URL.Query()
		if q.Get("school_id") != "600" || q.Get("entrance_year") != "2024" || q.Get("graduate_year") != "2027" {
			t.Fatal("saved account context not used")
		}
		if req.Header.Get("Accept-Encoding") == "gzip" {
			t.Fatal("manual gzip disables transparent decompression")
		}
		if req.URL.Path == "/signin40/homepage" {
			return reportResponse(`{"code":20000,"data":{"mark_list":[{"key":"42","value":"院区轮转"}]}}`), nil
		}
		if req.Method != "POST" || req.URL.Path != "/signin_rsa" {
			t.Fatal("wrong submission endpoint")
		}
		req.ParseForm()
		for key, want := range map[string]string{"remark": "42", "comment": "今日轮转说明", "file": "/upload/one.jpg,/upload/two.png", "province": "广东省", "city": "广州市", "address_name": "参考地点", "change_sign_resource": "0"} {
			if req.PostForm.Get(key) != want {
				t.Errorf("%s = %q; want %q", key, req.PostForm.Get(key), want)
			}
		}
		if req.PostForm.Get("latitude") == "23.123456" || req.PostForm.Get("longitude") == "113.654321" || req.PostForm.Get("latitude") == "" {
			t.Fatal("coordinates not RSA encrypted")
		}
		posted = true
		return reportResponse(`{"code":"20000","data":{}}`), nil
	})
	_, err := s.SignInWithOptions(context.Background(), "student-account", "广东省广州市番禺区参考地点", "参考地点", "23.123456", "113.654321", service.SignOptions{Remark: "42", Comment: "今日轮转说明", Photos: []string{"/upload/one.jpg", "/upload/two.png"}})
	if err != nil || !posted {
		t.Fatalf("submission failed: %v", err)
	}
}

func TestAutoSignPinsSchoolAndRemarkThroughPreflight(t *testing.T) {
	for _, mode := range []string{"school", "remark"} {
		t.Run(mode, func(t *testing.T) {
			posts := 0
			s := signFixture(func(req *http.Request) (*http.Response, error) {
				if req.Method == "POST" {
					posts++
				}
				return reportResponse(`{"code":20000,"data":{"mark_list":[{"key":"0","value":"下班"}]}}`), nil
			})
			if mode == "school" {
				reads := 0
				s.additionalUserData = func(string) (map[string]string, error) {
					reads++
					id := "600"
					if reads > 1 {
						id = "601"
					}
					return map[string]string{"school_id": id}, nil
				}
			}
			label := "下班"
			if mode == "remark" {
				label = "上班"
			}
			_, err := s.SignInWithOptions(context.Background(), "u", "广东省广州市参考地址", "参考地点", "23", "113", service.SignOptions{Remark: "0", ExpectedSchoolID: "600.0", ExpectedRemarkLabel: label})
			if !errors.Is(err, service.ErrSignInput) || posts != 0 {
				t.Fatalf("binding changed but submitted: %v posts=%d", err, posts)
			}
		})
	}
}

func TestAutoSignRechecksTaskBeforeActualSubmission(t *testing.T) {
	posts := 0
	s := signFixture(func(req *http.Request) (*http.Response, error) {
		if req.Method == "POST" {
			posts++
		}
		return reportResponse(`{"code":20000,"data":{"mark_list":[{"key":"0","value":"上班"}]}}`), nil
	})
	_, err := s.SignInWithOptions(context.Background(), "u", "广东省广州市参考地址", "参考地点", "23", "113", service.SignOptions{Remark: "0", BeforeSubmit: func() error { return service.ErrSignCancelled }})
	if !errors.Is(err, service.ErrSignCancelled) || posts != 0 {
		t.Fatalf("paused task submitted: %v posts=%d", err, posts)
	}
}

func TestSignRejectsUnavailableRemark(t *testing.T) {
	calls := 0
	s := signFixture(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != "GET" {
			t.Fatal("invalid remark was submitted")
		}
		return reportResponse(`{"code":20000,"data":{"mark_list":[{"key":0,"value":"上班"}]}}`), nil
	})
	_, err := s.SignInWithOptions(context.Background(), "u", "广东省广州市参考地点", "", "23", "113", service.SignOptions{Remark: "13"})
	if !errors.Is(err, service.ErrSignInput) || calls != 1 {
		t.Fatalf("unknown remark accepted: %v", err)
	}
}

func TestSignTransportErrorsHideCredentialsAndDoNotRetry(t *testing.T) {
	calls := 0
	s := signFixture(func(req *http.Request) (*http.Response, error) { calls++; return nil, errors.New(req.URL.String()) })
	_, err := s.Remarks(context.Background(), "u")
	if err == nil || strings.Contains(err.Error(), "private-token") || calls != 1 {
		t.Fatalf("error leaks credentials or retries: calls=%d", calls)
	}
}

func TestSignPhotoUploadUsesSignBusiness(t *testing.T) {
	s := signFixture(func(req *http.Request) (*http.Response, error) {
		if req.Method != "POST" || req.URL.Path != "/file/form" {
			t.Fatal("wrong photo upload endpoint")
		}
		if err := req.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		defer req.MultipartForm.RemoveAll()
		if req.FormValue("input_name") != "addFile" || req.FormValue("business") != "signin" {
			t.Fatal("photo uploaded as a report attachment")
		}
		file, _, err := req.FormFile("addFile[]")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		if !strings.HasPrefix(string(data), "\x89PNG\r\n\x1a\n") {
			t.Fatal("photo bytes changed")
		}
		return reportResponse(`{"code":20000,"data":{"uri":"/upload/photo.png"}}`), nil
	})
	uri, err := s.UploadPhoto(context.Background(), "u", "photo.png", strings.NewReader("\x89PNG\r\n\x1a\nphoto-data"))
	if err != nil || uri != "/upload/photo.png" {
		t.Fatalf("upload: %v", err)
	}
}

func TestSignRejectsNonImagesBeforeUpload(t *testing.T) {
	s := signFixture(func(*http.Request) (*http.Response, error) { t.Fatal("invalid image reached network"); return nil, nil })
	_, err := s.UploadPhoto(context.Background(), "u", "fake.png", strings.NewReader("not an image"))
	if !errors.Is(err, service.ErrSignInput) {
		t.Fatalf("fake image accepted: %v", err)
	}
}

func TestSignPhotoUploadNormalizesOfficialStorageURL(t *testing.T) {
	for _, host := range []string{"vanber-production.oss-cn-shanghai-internal.aliyuncs.com", "vanber-production.oss-cn-shanghai.aliyuncs.com"} {
		t.Run(host, func(t *testing.T) {
			s := signFixture(func(*http.Request) (*http.Response, error) {
				return reportResponse(`{"code":20000,"data":{"uri":"http://` + host + `/2222/SignInLog/2026-10-14/photo.jpg","name":"photo.jpg"}}`), nil
			})
			uri, err := s.UploadPhoto(context.Background(), "u", "photo.png", strings.NewReader("\x89PNG\r\n\x1a\nphoto-data"))
			if err != nil || uri != "/2222/SignInLog/2026-10-14/photo.jpg" {
				t.Fatalf("official upload response not normalized: uri=%q error=%v", uri, err)
			}
		})
	}
}

func TestSignPhotoUploadRejectsUntrustedStorageURL(t *testing.T) {
	for _, uri := range []string{"https://example.com/photo.jpg", "http://vanber-production.oss-cn-shanghai-internal.aliyuncs.com/photo.jpg?token=secret", "http://user@vanber-production.oss-cn-shanghai-internal.aliyuncs.com/photo.jpg"} {
		s := signFixture(func(*http.Request) (*http.Response, error) {
			return reportResponse(`{"code":20000,"data":{"uri":"` + uri + `"}}`), nil
		})
		if _, err := s.UploadPhoto(context.Background(), "u", "photo.png", strings.NewReader("\x89PNG\r\n\x1a\nphoto-data")); err == nil {
			t.Fatal("untrusted upload URL accepted")
		}
	}
}

func TestSignRegionExtraction(t *testing.T) {
	for _, test := range []struct{ address, province, city string }{{"北京市海淀区中关村", "北京市", "北京市"}, {"广西壮族自治区南宁市青秀区", "广西壮族自治区", "南宁市"}, {"广东省广州市番禺区", "广东省", "广州市"}} {
		province, city, err := extractProvinceAndCity(test.address)
		if err != nil || province != test.province || city != test.city {
			t.Fatalf("region %q: %q, %q, %v", test.address, province, city, err)
		}
	}
}
