package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// The signing key exists only in process memory. Restarting the service requires
// a fresh selection; neither candidate locations nor signatures are persisted.
var locationConfirmationKey = func() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("无法初始化地点确认")
	}
	return key
}()

func locationMAC(expiry, address, name, latitude, longitude string) []byte {
	payload, _ := json.Marshal([]string{expiry, address, name, latitude, longitude})
	mac := hmac.New(sha256.New, locationConfirmationKey)
	mac.Write(payload)
	return mac.Sum(nil)
}

func signLocation(place locationResult) locationResult {
	expires := strconv.FormatInt(time.Now().Add(30*time.Minute).Unix(), 10)
	place.ConfirmationToken = expires + "." + hex.EncodeToString(locationMAC(expires, place.Address, place.Name, place.Latitude, place.Longitude))
	return place
}

func verifyLocation(req SignRequest) bool {
	parts := strings.Split(req.ConfirmationToken, ".")
	if len(parts) != 2 {
		return false
	}
	expires, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || expires < time.Now().Unix() {
		return false
	}
	mac, err := hex.DecodeString(parts[1])
	return err == nil && hmac.Equal(mac, locationMAC(parts[0], req.Address, req.AddressName, req.Latitude, req.Longitude))
}
