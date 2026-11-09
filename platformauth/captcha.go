package platformauth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type CaptchaConfig struct {
	Enabled   bool   `json:"enabled"`
	Ready     bool   `json:"ready"`
	CaptchaID string `json:"captcha_id"`
	key       string
}
type CaptchaProof struct {
	LotNumber     string `json:"lot_number"`
	CaptchaOutput string `json:"captcha_output"`
	PassToken     string `json:"pass_token"`
	GenTime       string `json:"gen_time"`
}
type GeeTest struct{ Client *http.Client }

func (GeeTest) Config() CaptchaConfig {
	get := authConfig()
	c := CaptchaConfig{Enabled: !strings.EqualFold(get("GEETEST_ENABLED"), "false"), CaptchaID: get("GEETEST_CAPTCHA_ID"), key: get("GEETEST_CAPTCHA_KEY")}
	if c.key == "" {
		c.key = get("GEETEST_API_KEY")
	}
	c.Ready = c.Enabled && c.CaptchaID != "" && c.key != ""
	if !c.Ready {
		c.CaptchaID = ""
	}
	return c
}

// Only the configured server key signs the lot number; browser input cannot
// select a verification endpoint or supply its own captcha ID.
func (g GeeTest) verify(ctx context.Context, c CaptchaConfig, p CaptchaProof) error {
	for _, value := range []string{p.LotNumber, p.CaptchaOutput, p.PassToken, p.GenTime} {
		if strings.TrimSpace(value) == "" || len(value) > 4096 {
			return failure(400, "请先完成人机验证。")
		}
	}
	mac := hmac.New(sha256.New, []byte(c.key))
	mac.Write([]byte(p.LotNumber))
	form := url.Values{"lot_number": {p.LotNumber}, "captcha_output": {p.CaptchaOutput}, "pass_token": {p.PassToken}, "gen_time": {p.GenTime}, "sign_token": {hex.EncodeToString(mac.Sum(nil))}}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, "POST", "https://gcaptcha4.geetest.com/validate?captcha_id="+url.QueryEscape(c.CaptchaID), strings.NewReader(form.Encode()))
	if e != nil {
		return failure(503, "人机验证服务暂时不可用，请稍后重试。")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := g.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	res, e := client.Do(req)
	if e != nil {
		return failure(503, "人机验证服务暂时不可用，请稍后重试。")
	}
	defer res.Body.Close()
	var result struct {
		Status string `json:"status"`
		Result string `json:"result"`
	}
	decoder := json.NewDecoder(io.LimitReader(res.Body, 64<<10))
	if res.StatusCode != http.StatusOK || decoder.Decode(&result) != nil {
		return failure(503, "人机验证服务暂时不可用，请稍后重试。")
	}
	if result.Status != "success" || result.Result != "success" {
		return failure(400, "人机验证未通过或已过期，请重新验证。")
	}
	return nil
}

func (s *Store) SendVerifiedCode(ctx context.Context, email, purpose, ip string, proof CaptchaProof) error {
	if _, e := emailAddress(email); e != nil {
		return e
	}
	if purpose != "register" && purpose != "reset" {
		return failure(400, "验证码用途无效。")
	}
	if e := s.verifyCaptcha(ctx, ip, proof); e != nil {
		return e
	}
	return s.SendCode(ctx, email, purpose, ip)
}

func (s *Store) verifyCaptcha(ctx context.Context, ip string, proof CaptchaProof) error {
	c := s.Captcha.Config()
	if c.Enabled {
		if !c.Ready {
			return failure(503, "人机验证尚未配置，请联系管理员。")
		}
		if e := s.limit("captcha:"+ip, 20, 10*time.Minute); e != nil {
			return e
		}
		if e := s.Captcha.verify(ctx, c, proof); e != nil {
			return e
		}
		// A successful proof may authorize only one send, including concurrent
		// requests and requests handled after restarting this service.
		now := s.now().Unix()
		if _, e := s.db.Exec("DELETE FROM platform_captcha_used WHERE expires_at<=?", now); e != nil {
			return e
		}
		res, e := s.db.Exec("INSERT OR IGNORE INTO platform_captcha_used(digest,expires_at) VALUES(?,?)", digest(c.CaptchaID+":"+proof.LotNumber), now+600)
		if e != nil {
			return e
		}
		count, e := res.RowsAffected()
		if e != nil {
			return e
		}
		if count == 0 {
			return failure(400, "人机验证已使用，请重新验证。")
		}
	}
	return nil
}
