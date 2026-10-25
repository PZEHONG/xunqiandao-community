package platformauth

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
)

// StudentBinding resolves a previously verified school without accepting school
// overrides from the login form. Leading zeroes in student numbers are retained.
func (s *Store) StudentBinding(account string) (Binding, bool, error) {
	var binding Binding
	if !usernamePattern.MatchString(account) {
		return binding, false, nil
	}
	err := s.db.QueryRow(`SELECT b.account,b.school_id,b.school_name FROM platform_bindings b JOIN platform_users u ON u.id=b.user_id WHERE u.username=? AND b.account=u.username`, account).Scan(&binding.Account, &binding.SchoolID, &binding.SchoolName)
	if errors.Is(err, sql.ErrNoRows) {
		return binding, false, nil
	}
	return binding, err == nil, err
}

// LoginOfficial is the compatibility wrapper for callers without a context.
func (s *Store) LoginOfficial(account, schoolID, schoolName, oldToken, ip string, verify func() error) (User, string, error) {
	if verify == nil {
		return User{}, "", failure(400, "Missing credential verifier.")
	}
	return s.LoginOfficialContext(context.Background(), account, schoolID, schoolName, oldToken, ip, func(context.Context) error { return verify() })
}

// LoginOfficialContext serializes one account without blocking other logins or disable.
func (s *Store) LoginOfficialContext(ctx context.Context, account, schoolID, schoolName, oldToken, ip string, verify func(context.Context) error) (User, string, error) {
	return s.loginOfficialContext(ctx, account, schoolID, schoolName, oldToken, ip, verify)
}

func (s *Store) loginOfficialContext(ctx context.Context, account, schoolID, schoolName, oldToken, ip string, verify func(context.Context) error) (User, string, error) {
	ctx, cancel := context.WithTimeout(ctx, LoginTimeout)
	defer cancel()
	if !usernamePattern.MatchString(account) || schoolID == "" || len(schoolID) > 64 || schoolID != strings.TrimSpace(schoolID) || len(schoolName) > 256 || verify == nil {
		return User{}, "", failure(400, "请完整填写学校、学号和习讯云密码。")
	}
	if e := s.AllowLogin(account, ip); e != nil {
		return User{}, "", e
	}
	release, e := s.LockLogin(ctx, account)
	if e != nil {
		return User{}, "", e
	}
	defer release()
	if e := s.CheckAccountSchool(account, schoolID); e != nil {
		return User{}, "", e
	}
	var owner int64
	var boundSchool string
	e = s.db.QueryRow("SELECT user_id,school_id FROM platform_bindings WHERE account=?", account).Scan(&owner, &boundSchool)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return User{}, "", e
	}
	if owner != 0 && boundSchool != schoolID {
		return User{}, "", failure(409, "该学号已关联其他学校，请使用原讯签到账号登录检查学校设置。")
	}
	if owner == 0 {
		var count int
		if e = s.db.QueryRow("SELECT COUNT(*) FROM platform_users WHERE username=?", account).Scan(&count); e != nil {
			return User{}, "", e
		}
		if count > 0 {
			return User{}, "", failure(409, "该学号已有讯签到账号，请先使用讯签到密码登录并连接习讯云，之后即可直连登录。")
		}
	}
	if e = verify(ctx); e != nil {
		return User{}, "", failure(401, "习讯云登录失败，请检查学校、学号和密码；官方服务繁忙时请稍后重试。")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e = ctx.Err(); e != nil {
		return User{}, "", e
	}
	if e = s.CheckAccountSchool(account, schoolID); e != nil {
		return User{}, "", e
	}
	token, e := randomToken()
	if e != nil {
		return User{}, "", e
	}
	tx, e := s.db.Begin()
	if e != nil {
		return User{}, "", e
	}
	defer tx.Rollback()
	u := User{ID: owner, Username: account, StudentNumber: account}
	if owner == 0 {
		// Unusable password and non-email marker ensure direct login never
		// creates a usable local password or a fictitious recovery mailbox.
		result, err := tx.Exec("INSERT INTO platform_users(username,email,password_hash,created_at) VALUES(?,?,?,?)", account, "official:"+digest(token), "", s.now().Unix())
		if err != nil {
			return User{}, "", failure(409, "该学号已有账号，请重新登录。")
		}
		u.ID, e = result.LastInsertId()
		if e != nil {
			return User{}, "", e
		}
		if _, e = tx.Exec("INSERT INTO platform_official_users(user_id) VALUES(?)", u.ID); e != nil {
			return User{}, "", e
		}
	} else {
		e = tx.QueryRow(`SELECT u.username,CASE WHEN o.user_id IS NULL THEN u.email ELSE '' END FROM platform_users u LEFT JOIN platform_official_users o ON o.user_id=u.id WHERE u.id=?`, owner).Scan(&u.Username, &u.Email)
		if e != nil {
			return User{}, "", e
		}
		u.StudentNumber = u.Username
	}
	bindResult, e := tx.Exec(`INSERT INTO platform_bindings(account,user_id,school_id,school_name) VALUES(?,?,?,?) ON CONFLICT(account) DO UPDATE SET school_name=CASE WHEN excluded.school_name='' THEN school_name ELSE excluded.school_name END WHERE user_id=excluded.user_id AND school_id=excluded.school_id`, account, u.ID, schoolID, schoolName)
	if e != nil {
		return User{}, "", e
	}
	bound, e := bindResult.RowsAffected()
	if e != nil {
		return User{}, "", e
	}
	if bound != 1 {
		return User{}, "", failure(409, "账号关联已更新，请重新登录。")
	}
	if _, e = tx.Exec("INSERT INTO platform_sessions(token_hash,user_id,expires_at) VALUES(?,?,?)", digest(token), u.ID, s.now().Add(SessionAge).Unix()); e != nil {
		return User{}, "", e
	}
	if oldToken != "" {
		if _, e = tx.Exec("DELETE FROM platform_sessions WHERE token_hash=?", digest(oldToken)); e != nil {
			return User{}, "", e
		}
	}
	if e = tx.Commit(); e != nil {
		return User{}, "", e
	}
	return u, token, nil
}

func (s *Store) AllowOfficialSchoolSearch(ip string) error {
	return s.limit("official-schools:"+ip, 30, time.Minute)
}

// The legacy token table is keyed by account without school, so both login
// paths must refuse to overwrite an account belonging to another school.
// Callers hold the account login gate until verification and binding finish.
func (s *Store) CheckAccountSchool(account, schoolID string) error {
	expected, e := strconv.ParseFloat(schoolID, 64)
	if e != nil || math.IsNaN(expected) || math.IsInf(expected, 0) || expected <= 0 {
		return failure(400, "请选择有效学校。")
	}
	var boundSchool string
	e = s.db.QueryRow("SELECT school_id FROM platform_bindings WHERE account=?", account).Scan(&boundSchool)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	if e == nil && boundSchool != schoolID {
		return failure(409, "该学号已关联其他学校，无法覆盖原账号的学校和凭证。")
	}
	var exists int
	if e = s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'").Scan(&exists); e != nil {
		return e
	}
	if exists == 0 {
		return nil
	}
	var stored sql.NullFloat64
	e = s.db.QueryRow("SELECT school_id FROM users WHERE account=?", account).Scan(&stored)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if stored.Valid && stored.Float64 > 0 && stored.Float64 != expected {
		return failure(409, "本机已有该学号在其他学校的账号记录，无法覆盖；请检查所选学校。")
	}
	return nil
}
