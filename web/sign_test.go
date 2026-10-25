package web

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestSignForwardsRemarkCommentAndPhotos(t *testing.T) {
	s := &privacySignStub{}
	place := signLocation(locationResult{Address: "广东省广州市参考地点", Name: "参考地点", Latitude: "23", Longitude: "113"})
	payload := map[string]interface{}{"account": "u", "address": place.Address, "address_name": place.Name, "latitude": place.Latitude, "longitude": place.Longitude, "location_confirmed": true, "confirmation_token": place.ConfirmationToken, "remark": "42", "comment": "轮转说明", "photos": []string{"/upload/test.jpg"}}
	data, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	authenticatedRouter(t, &Handlers{SignService: s}).ServeHTTP(w, httptest.NewRequest("POST", "/api/sign", bytes.NewReader(data)))
	if w.Code != 200 || s.calls != 1 || s.options.Remark != "42" || s.options.Comment != "轮转说明" || len(s.options.Photos) != 1 {
		t.Fatalf("sign fields were dropped: status=%d options=%+v", w.Code, s.options)
	}
}

func TestSignBodyLimit(t *testing.T) {
	w := httptest.NewRecorder()
	authenticatedRouter(t, &Handlers{SignService: &privacySignStub{}}).ServeHTTP(w, httptest.NewRequest("POST", "/api/sign", bytes.NewReader(bytes.Repeat([]byte("x"), 256<<10))))
	if w.Code != 400 {
		t.Fatalf("oversized JSON accepted: %d", w.Code)
	}
}
