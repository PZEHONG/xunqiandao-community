// Package platformauth manages workspace identities independently of school accounts.
package platformauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const SessionAge = 365 * 24 * time.Hour

type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string                 { return e.Message }
func failure(status int, message string) error { return &Error{status, message} }

type User struct {
	ID            int64  `json:"id"`
	Username      string `json:"username"`
	Email         string `json:"email"`
	StudentNumber string `json:"student_number"`
}
type Binding struct {
	Account    string `json:"account"`
	SchoolID   string `json:"school_id"`
	SchoolName string `json:"school_name"`
}
type Store struct {
	Captcha GeeTest
	db      *sql.DB
	mail    Mailer
	now     func() time.Time
	mu      sync.Mutex
	// BindMu is retained for source compatibility; login uses account gates.
	BindMu     sync.Mutex
	loginMu    sync.Mutex
	loginGates map[string]*accountLoginGate
	dummyHash  []byte
}

func New(db *sql.DB, mailer Mailer) (*Store, error) {
	if db == nil {
		return nil, errors.New("authentication database unavailable")
	}
	_, err := db.Exec(`
 CREATE TABLE IF NOT EXISTS platform_users (id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT NOT NULL COLLATE NOCASE UNIQUE, email TEXT NOT NULL COLLATE NOCASE UNIQUE, password_hash TEXT NOT NULL, created_at INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS platform_sessions (token_hash TEXT PRIMARY KEY, user_id INTEGER NOT NULL, expires_at INTEGER NOT NULL);
 CREATE INDEX IF NOT EXISTS platform_sessions_user ON platform_sessions(user_id);
 CREATE TABLE IF NOT EXISTS platform_codes (email TEXT NOT NULL, purpose TEXT NOT NULL, digest TEXT NOT NULL, nonce TEXT NOT NULL, expires_at INTEGER NOT NULL, sent_at INTEGER NOT NULL, attempts INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(email,purpose));
 CREATE TABLE IF NOT EXISTS platform_limits (key TEXT PRIMARY KEY, hits INTEGER NOT NULL, expires_at INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS platform_captcha_used (digest TEXT PRIMARY KEY, expires_at INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS platform_bindings (account TEXT PRIMARY KEY, user_id INTEGER NOT NULL, school_id TEXT NOT NULL, school_name TEXT NOT NULL DEFAULT '');
 CREATE INDEX IF NOT EXISTS platform_bindings_user ON platform_bindings(user_id);
 CREATE TABLE IF NOT EXISTS platform_official_users (user_id INTEGER PRIMARY KEY);
 CREATE TABLE IF NOT EXISTS platform_consents (user_id INTEGER NOT NULL, version TEXT NOT NULL, accepted_at INTEGER NOT NULL, PRIMARY KEY(user_id,version));
 `)
	if err != nil {
		return nil, err
	}
	dummy, err := bcrypt.GenerateFromPassword([]byte("workspace-dummy-password"), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return &Store{db: db, mail: mailer, now: time.Now, dummyHash: dummy}, nil
}
func (s *Store) MailReady() bool { return s.mail != nil && s.mail.Ready() }
func emailAddress(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	address, e := mail.ParseAddress(value)
	if e != nil || address.Address != value || len(value) > 254 || !strings.Contains(strings.Split(value, "@")[len(strings.Split(value, "@"))-1], ".") {
		return "", failure(400, "请输入有效的邮箱地址。")
	}
	return value, nil
}
func passwordValid(password string) bool { return len(password) >= 8 && len(password) <= 72 }

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]{3,24}$`)

func digest(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
func randomToken() (string, error) {
	var bytes [32]byte
	_, e := rand.Read(bytes[:])
	return hex.EncodeToString(bytes[:]), e
}
func (s *Store) limit(key string, max int, window time.Duration) error {
	now := s.now().Unix()
	// Keys are digested so throttling metadata does not retain raw addresses or IPs.
	key = digest(key)
	var hits int
	e := s.db.QueryRow(`INSERT INTO platform_limits(key,hits,expires_at) VALUES(?,1,?) ON CONFLICT(key) DO UPDATE SET hits=CASE WHEN expires_at<=? THEN 1 ELSE hits+1 END, expires_at=CASE WHEN expires_at<=? THEN excluded.expires_at ELSE expires_at END RETURNING hits`, key, now+int64(window.Seconds()), now, now).Scan(&hits)
	if e != nil {
		return e
	}
	if hits > max {
		return failure(429, "操作过于频繁，请稍后再试。")
	}
	return nil
}
func (s *Store) SendCode(ctx context.Context, email, purpose, ip string) error {
	email, e := emailAddress(email)
	if e != nil {
		return e
	}
	if purpose != "register" && purpose != "reset" {
		return failure(400, "验证码用途无效。")
	}
	if !s.MailReady() {
		return failure(503, "邮件服务尚未配置，请联系管理员填写 SMTP 配置后重试。")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().Unix()
	var sent int64
	e = s.db.QueryRow("SELECT sent_at FROM platform_codes WHERE email=? AND purpose=?", email, purpose).Scan(&sent)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	if sent > now-60 {
		return failure(429, "验证码已发送，请在 60 秒后重试。")
	}
	if e = s.limit("mail:"+ip, 5, 10*time.Minute); e != nil {
		return e
	}
	var count int
	if e = s.db.QueryRow("SELECT COUNT(*) FROM platform_users WHERE email=?", email).Scan(&count); e != nil {
		return e
	}
	if purpose == "register" && count > 0 {
		return failure(409, "该邮箱已注册，请登录或找回密码。")
	}
	// Password recovery responds identically for unknown mailboxes, with the same cooldown.
	number, e := rand.Int(rand.Reader, big.NewInt(1000000))
	if e != nil {
		return e
	}
	code := fmt.Sprintf("%06d", number.Int64())
	nonce, e := randomToken()
	if e != nil {
		return e
	}
	if purpose != "reset" || count > 0 {
		if e = s.mail.SendCode(ctx, email, code, purpose); e != nil {
			return failure(502, "验证码发送失败，请检查邮件服务配置后重试。")
		}
	}
	_, e = s.db.Exec(`INSERT INTO platform_codes(email,purpose,digest,nonce,expires_at,sent_at,attempts) VALUES(?,?,?,?,?,?,0) ON CONFLICT(email,purpose) DO UPDATE SET digest=excluded.digest,nonce=excluded.nonce,expires_at=excluded.expires_at,sent_at=excluded.sent_at,attempts=0`, email, purpose, digest(nonce+code), nonce, now+600, now)
	if e == nil {
		s.db.Exec("DELETE FROM platform_limits WHERE expires_at<?", now)
		s.db.Exec("DELETE FROM platform_codes WHERE expires_at<?", now-86400)
		s.db.Exec("DELETE FROM platform_sessions WHERE expires_at<?", now)
	}
	return e
}

// checkCode runs under mu; attempts are committed even when the operation fails.
func (s *Store) checkCode(email, purpose, code string) error {
	var stored, nonce string
	var expiry int64
	var attempts int
	e := s.db.QueryRow("SELECT digest,nonce,expires_at,attempts FROM platform_codes WHERE email=? AND purpose=?", email, purpose).Scan(&stored, &nonce, &expiry, &attempts)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	if errors.Is(e, sql.ErrNoRows) || expiry <= s.now().Unix() || attempts >= 5 {
		return failure(400, "验证码已失效，请重新获取。")
	}
	if len(code) != 6 || subtle.ConstantTimeCompare([]byte(stored), []byte(digest(nonce+code))) != 1 {
		if _, e = s.db.Exec("UPDATE platform_codes SET attempts=attempts+1 WHERE email=? AND purpose=?", email, purpose); e != nil {
			return e
		}
		return failure(400, "验证码不正确，请检查邮箱后重试。")
	}
	return nil
}
func (s *Store) Register(username, email, password, code string) (User, error) {
	username = strings.TrimSpace(username)
	email, e := emailAddress(email)
	if e != nil {
		return User{}, e
	}
	if !usernamePattern.MatchString(username) {
		return User{}, failure(400, "学号需为 3–24 位字母、数字或下划线。")
	}
	if !passwordValid(password) {
		return User{}, failure(400, "密码需为 8–72 字节，请调整长度。")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e = s.checkCode(email, "register", code); e != nil {
		return User{}, e
	}
	var existing int
	if e = s.db.QueryRow("SELECT COUNT(*) FROM platform_users WHERE username=? OR email=?", username, email).Scan(&existing); e != nil {
		return User{}, e
	}
	if existing > 0 {
		return User{}, failure(409, "学号或邮箱已注册，请更换或直接登录。")
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if e != nil {
		return User{}, e
	}
	tx, e := s.db.Begin()
	if e != nil {
		return User{}, e
	}
	defer tx.Rollback()
	result, e := tx.Exec("INSERT INTO platform_users(username,email,password_hash,created_at) VALUES(?,?,?,?)", username, email, string(hash), s.now().Unix())
	if e != nil {
		return User{}, failure(409, "学号或邮箱已注册，请更换或直接登录。")
	}
	id, e := result.LastInsertId()
	if e != nil {
		return User{}, e
	}
	if _, e = tx.Exec("DELETE FROM platform_codes WHERE email=? AND purpose='register'", email); e != nil {
		return User{}, e
	}
	if e = tx.Commit(); e != nil {
		return User{}, e
	}
	return User{ID: id, Username: username, Email: email, StudentNumber: username}, nil
}
func (s *Store) Login(identity, password, ip string) (User, string, error) {
	identity = strings.TrimSpace(identity)
	if !usernamePattern.MatchString(identity) || !passwordValid(password) {
		return User{}, "", failure(401, "学号或密码不正确；首次使用请先通过习讯云登录。")
	}
	if e := s.AllowLogin(identity, ip); e != nil {
		return User{}, "", e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var u User
	var hash string
	e := s.db.QueryRow("SELECT id,username,email,password_hash FROM platform_users WHERE username=?", identity).Scan(&u.ID, &u.Username, &u.Email, &hash)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return User{}, "", e
	}
	check := []byte(hash)
	if errors.Is(e, sql.ErrNoRows) {
		check = s.dummyHash
	}
	if bcrypt.CompareHashAndPassword(check, []byte(password)) != nil || u.ID == 0 {
		return User{}, "", failure(401, "学号或密码不正确；首次使用请先通过习讯云登录。")
	}
	token, e := randomToken()
	if e != nil {
		return User{}, "", e
	}
	_, e = s.db.Exec("INSERT INTO platform_sessions(token_hash,user_id,expires_at) VALUES(?,?,?)", digest(token), u.ID, s.now().Add(SessionAge).Unix())
	if e != nil {
		return User{}, "", e
	}

	u.StudentNumber = u.Username
	return u, token, nil
}
func (s *Store) Self(token string) (User, error) {
	if len(token) != 64 {
		return User{}, failure(401, "请先登录讯签到。")
	}
	var u User
	e := s.db.QueryRow(`SELECT u.id,u.username,CASE WHEN o.user_id IS NULL THEN u.email ELSE '' END FROM platform_sessions s JOIN platform_users u ON u.id=s.user_id LEFT JOIN platform_official_users o ON o.user_id=u.id WHERE s.token_hash=? AND s.expires_at>?`, digest(token), s.now().Unix()).Scan(&u.ID, &u.Username, &u.Email)
	if errors.Is(e, sql.ErrNoRows) {
		return User{}, failure(401, "登录已过期，请重新登录。")
	}
	if e != nil {
		return User{}, e
	}
	// Extend only a still-valid session; logout and expiry must never be undone.
	result, e := s.db.Exec("UPDATE platform_sessions SET expires_at=? WHERE token_hash=? AND expires_at>?", s.now().Add(SessionAge).Unix(), digest(token), s.now().Unix())
	if e != nil {
		return User{}, e
	}
	count, e := result.RowsAffected()
	if e != nil {
		return User{}, e
	}
	if count != 1 {
		return User{}, failure(401, "登录已过期，请重新登录。")
	}
	u.StudentNumber = u.Username
	return u, e
}
func (s *Store) Logout(token string) error {
	_, e := s.db.Exec("DELETE FROM platform_sessions WHERE token_hash=?", digest(token))
	return e
}
func (s *Store) Reset(email, code, password string) error {
	email, e := emailAddress(email)
	if e != nil {
		return e
	}
	if !passwordValid(password) {
		return failure(400, "密码需为 8–72 字节，请调整长度。")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e = s.checkCode(email, "reset", code); e != nil {
		return e
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	result, e := tx.Exec("UPDATE platform_users SET password_hash=? WHERE email=?", string(hash), email)
	if e != nil {
		return e
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return failure(400, "验证码已失效，请重新获取。")
	}
	if _, e = tx.Exec("DELETE FROM platform_sessions WHERE user_id IN (SELECT id FROM platform_users WHERE email=?)", email); e != nil {
		return e
	}
	if _, e = tx.Exec("DELETE FROM platform_codes WHERE email=? AND purpose='reset'", email); e != nil {
		return e
	}
	var username string
	if e = tx.QueryRow("SELECT username FROM platform_users WHERE email=?", email).Scan(&username); e != nil {
		return e
	}
	if _, e = tx.Exec("DELETE FROM platform_limits WHERE key IN (?,?,?)", digest("login-user:"+email), digest("login-user:"+strings.ToLower(username)), digest("upstream-login-user:"+strings.ToLower(username))); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) CanBind(userID int64, account string) bool {
	var owner int64
	e := s.db.QueryRow("SELECT user_id FROM platform_bindings WHERE account=?", account).Scan(&owner)
	return errors.Is(e, sql.ErrNoRows) || (e == nil && owner == userID)
}
func (s *Store) Owns(userID int64, account string) bool {
	var owner int64
	e := s.db.QueryRow("SELECT user_id FROM platform_bindings WHERE account=?", account).Scan(&owner)
	return e == nil && owner == userID
}
func (s *Store) Bind(userID int64, account, schoolID, schoolName string) error {
	result, e := s.db.Exec(`INSERT INTO platform_bindings(account,user_id,school_id,school_name) VALUES(?,?,?,?) ON CONFLICT(account) DO UPDATE SET school_id=excluded.school_id,school_name=excluded.school_name WHERE user_id=excluded.user_id`, account, userID, schoolID, schoolName)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n == 0 {
		return failure(403, "该习讯云账号已绑定其他系统用户。")
	}
	return nil
}
func (s *Store) Bindings(userID int64) ([]Binding, error) {
	rows, e := s.db.Query("SELECT account,school_id,school_name FROM platform_bindings WHERE user_id=? ORDER BY rowid DESC", userID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	result := []Binding{}
	for rows.Next() {
		var b Binding
		if e = rows.Scan(&b.Account, &b.SchoolID, &b.SchoolName); e != nil {
			return nil, e
		}
		result = append(result, b)
	}
	return result, rows.Err()
}
