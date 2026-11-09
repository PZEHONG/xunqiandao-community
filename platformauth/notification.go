package platformauth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"time"
)

type NotificationEmail struct {
	Email     string `json:"email"`
	Enabled   bool   `json:"enabled"`
	Version   int64  `json:"version"`
	Verified  bool   `json:"verified"`
	UpdatedAt int64  `json:"updated_at"`
}

func (s *Store) NotificationEmail(userID int64) (NotificationEmail, bool, error) {
	var n NotificationEmail
	var enabled int
	var verifiedAt int64
	e := s.db.QueryRow(`SELECT email,enabled,version,verified_at,updated_at FROM platform_notification_emails WHERE user_id=?`, userID).Scan(&n.Email, &enabled, &n.Version, &verifiedAt, &n.UpdatedAt)
	if errors.Is(e, sql.ErrNoRows) {
		return NotificationEmail{}, false, nil
	}
	if e != nil {
		return NotificationEmail{}, false, e
	}
	n.Enabled, n.Verified = enabled == 1, verifiedAt > 0
	return n, true, nil
}

func (s *Store) SendVerifiedNotificationCode(ctx context.Context, userID int64, email, ip string, proof CaptchaProof) error {
	if _, e := emailAddress(email); e != nil {
		return e
	}
	if e := s.verifyCaptcha(ctx, ip, proof); e != nil {
		return e
	}
	return s.SendNotificationCode(ctx, userID, email, ip)
}

// SendNotificationCode stores a code bound to this user and address. It cannot
// be reused for another purpose or another account.
func (s *Store) SendNotificationCode(ctx context.Context, userID int64, email, ip string) error {
	email, e := emailAddress(email)
	if e != nil {
		return e
	}
	if !s.MailReady() {
		return failure(503, "邮件服务尚未配置，请联系管理员填写 SMTP 配置后重试。")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().Unix()
	var sent int64
	e = s.db.QueryRow(`SELECT sent_at FROM platform_notification_codes WHERE user_id=? AND email=? AND purpose='notify'`, userID, email).Scan(&sent)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	if sent > now-60 {
		return failure(429, "验证码已发送，请在 60 秒后重试。")
	}
	if e = s.limit("notify-mail:"+ip, 5, 10*time.Minute); e != nil {
		return e
	}
	if e = s.limit("notify-user:"+fmt.Sprint(userID), 5, 10*time.Minute); e != nil {
		return e
	}
	number, e := rand.Int(rand.Reader, big.NewInt(1000000))
	if e != nil {
		return e
	}
	code := fmt.Sprintf("%06d", number.Int64())
	nonce, e := randomToken()
	if e != nil {
		return e
	}
	if e = s.mail.SendCode(ctx, email, code, "notify"); e != nil {
		return failure(502, "验证码发送失败，请检查邮件服务配置后重试。")
	}
	_, e = s.db.Exec(`INSERT INTO platform_notification_codes(user_id,email,purpose,digest,nonce,expires_at,sent_at,attempts) VALUES(?,?,'notify',?,?,?,?,0) ON CONFLICT(user_id,email,purpose) DO UPDATE SET digest=excluded.digest,nonce=excluded.nonce,expires_at=excluded.expires_at,sent_at=excluded.sent_at,attempts=0`, userID, email, digest(nonce+code), nonce, now+600, now)
	if e == nil {
		s.db.Exec(`DELETE FROM platform_notification_codes WHERE expires_at<?`, now-86400)
	}
	return e
}

func (s *Store) BindNotificationEmail(userID int64, email, code string) (NotificationEmail, error) {
	email, e := emailAddress(email)
	if e != nil {
		return NotificationEmail{}, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().Unix()
	tx, e := s.db.Begin()
	if e != nil {
		return NotificationEmail{}, e
	}
	defer tx.Rollback()
	var stored, nonce string
	var expiry int64
	var attempts int
	e = tx.QueryRow(`SELECT digest,nonce,expires_at,attempts FROM platform_notification_codes WHERE user_id=? AND email=? AND purpose='notify'`, userID, email).Scan(&stored, &nonce, &expiry, &attempts)
	if errors.Is(e, sql.ErrNoRows) {
		return NotificationEmail{}, failure(400, "验证码已失效，请重新获取。")
	}
	if e != nil {
		return NotificationEmail{}, e
	}
	if expiry <= now || attempts >= 5 {
		return NotificationEmail{}, failure(400, "验证码已失效，请重新获取。")
	}
	if len(code) != 6 || subtle.ConstantTimeCompare([]byte(stored), []byte(digest(nonce+code))) != 1 {
		if _, e = tx.Exec(`UPDATE platform_notification_codes SET attempts=attempts+1 WHERE user_id=? AND email=? AND purpose='notify'`, userID, email); e != nil {
			return NotificationEmail{}, e
		}
		if e = tx.Commit(); e != nil {
			return NotificationEmail{}, e
		}
		return NotificationEmail{}, failure(400, "验证码不正确，请检查邮箱后重试。")
	}
	var version int64
	// Re-verifying the same address keeps its version; changing it invalidates
	// any unsent events created for the previous address.
	e = tx.QueryRow(`INSERT INTO platform_notification_emails(user_id,email,enabled,version,verified_at,updated_at) VALUES(?,?,1,1,?,?)
 ON CONFLICT(user_id) DO UPDATE SET email=excluded.email,
 version=CASE WHEN email<>excluded.email THEN version+1 ELSE version END,
 enabled=1,verified_at=excluded.verified_at,updated_at=excluded.updated_at
 RETURNING version`, userID, email, now, now).Scan(&version)
	if e != nil {
		return NotificationEmail{}, e
	}
	if _, e = tx.Exec(`DELETE FROM platform_notification_codes WHERE user_id=? AND email=? AND purpose='notify'`, userID, email); e != nil {
		return NotificationEmail{}, e
	}
	if e = tx.Commit(); e != nil {
		return NotificationEmail{}, e
	}
	return NotificationEmail{Email: email, Enabled: true, Version: version, Verified: true, UpdatedAt: now}, nil
}

func (s *Store) UnbindNotificationEmail(userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`DELETE FROM platform_notification_emails WHERE user_id=?`, userID); e != nil {
		return e
	}
	if _, e = tx.Exec(`DELETE FROM platform_notification_codes WHERE user_id=?`, userID); e != nil {
		return e
	}
	return tx.Commit()
}

func (s *Store) SetNotificationEnabled(userID int64, enabled bool) (NotificationEmail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().Unix()
	result, e := s.db.Exec(`UPDATE platform_notification_emails SET enabled=?,updated_at=? WHERE user_id=?`, enabled, now, userID)
	if e != nil {
		return NotificationEmail{}, e
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return NotificationEmail{}, failure(404, "尚未绑定签到提醒邮箱。")
	}
	n, _, e := s.NotificationEmail(userID)
	return n, e
}
