// Package autosign owns explicitly confirmed daily tasks. It never imports or
// activates legacy schedules, and never stores positions on user accounts.
package autosign

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var Shanghai = time.FixedZone("Asia/Shanghai", 8*60*60)
var ErrConflict = errors.New("任务已变更，请刷新后重新确认")
var ErrNotFound = errors.New("任务不存在或不属于当前账号")

type Task struct {
	ID          int64     `json:"id"`
	Account     string    `json:"account"`
	SchoolID    string    `json:"school_id"`
	Name        string    `json:"name"`
	Address     string    `json:"address"`
	AddressName string    `json:"address_name"`
	Latitude    string    `json:"latitude"`
	Longitude   string    `json:"longitude"`
	Remark      string    `json:"remark"`
	RemarkLabel string    `json:"remark_label"`
	Comment     string    `json:"comment"`
	Time        string    `json:"time"`
	Enabled     bool      `json:"enabled"`
	Revision    int64     `json:"revision"`
	ConfirmedAt time.Time `json:"confirmed_at"`
	NotBefore   time.Time `json:"-"`
	LastRun     *Run      `json:"last_run,omitempty"`
	Precheck    *Precheck `json:"precheck,omitempty"`
}
type Run struct {
	ID                   int64  `json:"id"`
	TaskID               int64  `json:"task_id"`
	Account              string `json:"account"`
	Name                 string `json:"name"`
	Revision             int64  `json:"revision"`
	Date                 string `json:"date"`
	ScheduledTime        string `json:"scheduled_time"`
	StartedAt            string `json:"started_at"`
	FinishedAt           string `json:"finished_at"`
	Status               string `json:"status"`
	Message              string `json:"message"`
	Stage                string `json:"stage,omitempty"`
	FailureKind          string `json:"failure_kind,omitempty"`
	VerificationAttempts int    `json:"verification_attempts,omitempty"`
	NotificationState    string `json:"notification_state,omitempty"`
}

// Precheck is the persisted result of the read-only check before the planned
// time. It never submits a sign-in.
type Precheck struct {
	TaskID    int64  `json:"task_id"`
	Date      string `json:"date"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	CheckedAt string `json:"checked_at"`
	NextAt    string `json:"next_at,omitempty"`
	Attempts  int    `json:"attempts"`
}
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("任务数据库不可用")
	}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS auto_sign_tasks(
 id INTEGER PRIMARY KEY AUTOINCREMENT,account TEXT NOT NULL,config TEXT NOT NULL,
 enabled INTEGER NOT NULL,revision INTEGER NOT NULL,confirmed_at TEXT NOT NULL,not_before TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS auto_sign_tasks_account ON auto_sign_tasks(account);
 CREATE TABLE IF NOT EXISTS auto_sign_runs(
 id INTEGER PRIMARY KEY AUTOINCREMENT,task_id INTEGER NOT NULL,account TEXT NOT NULL,name TEXT NOT NULL,
 revision INTEGER NOT NULL,date TEXT NOT NULL,scheduled_time TEXT NOT NULL,started_at TEXT NOT NULL,
 finished_at TEXT NOT NULL DEFAULT '',status TEXT NOT NULL,message TEXT NOT NULL DEFAULT '',
 stage TEXT NOT NULL DEFAULT '',failure_kind TEXT NOT NULL DEFAULT '',verification_attempts INTEGER NOT NULL DEFAULT 0,
 verification_done INTEGER NOT NULL DEFAULT 0,next_verify_at TEXT NOT NULL DEFAULT '',UNIQUE(task_id,date));
 CREATE INDEX IF NOT EXISTS auto_sign_runs_account ON auto_sign_runs(account,id);
 CREATE INDEX IF NOT EXISTS auto_sign_runs_date ON auto_sign_runs(date,status);
 CREATE INDEX IF NOT EXISTS auto_sign_runs_task ON auto_sign_runs(task_id,id);
 CREATE TABLE IF NOT EXISTS auto_sign_prechecks(
 task_id INTEGER NOT NULL,date TEXT NOT NULL,status TEXT NOT NULL DEFAULT '',message TEXT NOT NULL DEFAULT '',
 attempts INTEGER NOT NULL DEFAULT 0,checked_at TEXT NOT NULL DEFAULT '',next_at TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(task_id,date));
 ` + NotificationSchema)
	if err != nil {
		return nil, err
	}
	if err = migrateRuns(db); err != nil {
		return nil, err
	}
	migration, err := db.Exec("UPDATE auto_sign_runs SET status='failed', message=CASE WHEN status='unknown' THEN '历史执行未确认成功，按失败记录；可能已提交，请先查看官方签到记录，系统不会自动重试' ELSE message END WHERE status IN ('unknown','skipped')")
	if err != nil {
		return nil, err
	}
	if count, _ := migration.RowsAffected(); count > 0 {
		log.Printf("[autosign] stage=migrate status=success converted_runs=%d", count)
	}
	return &Store{db: db}, nil
}

// NotificationSchema is shared with platformauth so the queue and the
// verification table exist for both the scheduler and the account service.
const NotificationSchema = `
CREATE TABLE IF NOT EXISTS platform_bindings (account TEXT PRIMARY KEY, user_id INTEGER NOT NULL, school_id TEXT NOT NULL DEFAULT '', school_name TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS platform_bindings_user ON platform_bindings(user_id);
CREATE TABLE IF NOT EXISTS platform_notification_emails(
 user_id INTEGER PRIMARY KEY,email TEXT NOT NULL COLLATE NOCASE,enabled INTEGER NOT NULL DEFAULT 1,
 version INTEGER NOT NULL DEFAULT 1,verified_at INTEGER NOT NULL,updated_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS auto_sign_notifications(
 id INTEGER PRIMARY KEY AUTOINCREMENT,run_id INTEGER NOT NULL UNIQUE,task_id INTEGER NOT NULL,
 account TEXT NOT NULL,name TEXT NOT NULL,date TEXT NOT NULL,scheduled_time TEXT NOT NULL,result TEXT NOT NULL,
 user_id INTEGER NOT NULL DEFAULT 0,email TEXT NOT NULL DEFAULT '',email_version INTEGER NOT NULL DEFAULT 0,
 state TEXT NOT NULL,reason TEXT NOT NULL DEFAULT '',attempts INTEGER NOT NULL DEFAULT 0,
 last_error TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL,next_attempt_at TEXT NOT NULL DEFAULT '',
 sent_at TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS auto_sign_notifications_state ON auto_sign_notifications(state,next_attempt_at);
`

func migrateRuns(db *sql.DB) error {
	rows, e := db.Query(`PRAGMA table_info(auto_sign_runs)`)
	if e != nil {
		return e
	}
	found := map[string]bool{}
	for rows.Next() {
		var n, notnull, pk int
		var name, kind string
		var def any
		if e = rows.Scan(&n, &name, &kind, &notnull, &def, &pk); e != nil {
			rows.Close()
			return e
		}
		found[name] = true
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	columns := []string{
		`ALTER TABLE auto_sign_runs ADD COLUMN stage TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE auto_sign_runs ADD COLUMN failure_kind TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE auto_sign_runs ADD COLUMN verification_attempts INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE auto_sign_runs ADD COLUMN verification_done INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE auto_sign_runs ADD COLUMN next_verify_at TEXT NOT NULL DEFAULT ''`,
	}
	for i, name := range []string{"stage", "failure_kind", "verification_attempts", "verification_done", "next_verify_at"} {
		if !found[name] {
			if _, e = db.Exec(columns[i]); e != nil {
				return e
			}
		}
	}
	return nil
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func (t Task) NextRun(now time.Time) time.Time {
	n := now.In(Shanghai)
	h, _ := strconv.Atoi(strings.Split(t.Time, ":")[0])
	m := 0
	if p := strings.Split(t.Time, ":"); len(p) == 2 {
		m, _ = strconv.Atoi(p[1])
	}
	next := time.Date(n.Year(), n.Month(), n.Day(), h, m, 0, 0, Shanghai)
	if !next.After(n) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
func (s *Store) School(account string) (string, error) {
	var id string
	var token string
	err := s.db.QueryRow(`SELECT CAST(school_id AS TEXT),COALESCE(token,'') FROM users WHERE account=?`, account).Scan(&id, &token)
	if err != nil || id == "" || id == "0.0" || token == "" {
		return "", errors.New("请先连接此账号并确认学校")
	}
	return id, nil
}
func validate(t Task) error {
	clock, e := time.Parse("15:04", t.Time)
	if e != nil || clock.Format("15:04") != t.Time {
		return errors.New("请选择有效的每日时间（HH:MM）")
	}
	if strings.TrimSpace(t.Account) == "" || strings.TrimSpace(t.SchoolID) == "" || strings.TrimSpace(t.Name) == "" || utf8.RuneCountInString(t.Name) > 60 {
		return errors.New("任务名称、账号或学校信息不完整")
	}
	if strings.TrimSpace(t.Address) == "" || utf8.RuneCountInString(t.Address) > 500 || utf8.RuneCountInString(t.AddressName) > 200 {
		return errors.New("请确认完整地点")
	}
	for _, c := range []struct {
		v   string
		max float64
	}{{t.Latitude, 90}, {t.Longitude, 180}} {
		f, e := strconv.ParseFloat(c.v, 64)
		if e != nil || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > c.max {
			return errors.New("地点坐标无效")
		}
	}
	if t.Remark == "" || t.RemarkLabel == "" || len(t.Remark) > 80 || utf8.RuneCountInString(t.Comment) > 1000 {
		return errors.New("签到类型无效或说明超过 1000 字")
	}
	return nil
}
func (s *Store) Save(t Task, now time.Time) (Task, error) {
	if err := validate(t); err != nil {
		return Task{}, err
	}
	t.Name = strings.TrimSpace(t.Name)
	t.LastRun = nil
	t.ConfirmedAt = now
	t.NotBefore = now
	payload, err := json.Marshal(t)
	if err != nil {
		return Task{}, err
	}
	if t.ID == 0 {
		var count int
		if err = s.db.QueryRow("SELECT count(*) FROM auto_sign_tasks WHERE account=?", t.Account).Scan(&count); err != nil {
			return Task{}, err
		}
		if count >= 50 {
			return Task{}, errors.New("每个账号最多 50 个每日任务")
		}
		var result sql.Result
		result, err = s.db.Exec(`INSERT INTO auto_sign_tasks(account,config,enabled,revision,confirmed_at,not_before) VALUES(?,?,?,1,?,?)`, t.Account, string(payload), t.Enabled, stamp(now), stamp(now))
		if err == nil {
			if n, e := result.RowsAffected(); e != nil {
				return Task{}, e
			} else if n != 1 {
				return Task{}, ErrConflict
			}
			t.ID, err = result.LastInsertId()
			t.Revision = 1
		}
	} else {
		var result sql.Result
		result, err = s.db.Exec(`UPDATE auto_sign_tasks SET config=?,enabled=?,revision=revision+1,confirmed_at=?,not_before=? WHERE id=? AND account=? AND revision=?`, string(payload), t.Enabled, stamp(now), stamp(now), t.ID, t.Account, t.Revision)
		if err == nil {
			n, _ := result.RowsAffected()
			if n != 1 {
				return Task{}, ErrConflict
			}
			t.Revision++
		}
	}
	return t, err
}
func (s *Store) Tasks(account string) ([]Task, error) {
	query := `SELECT id,account,config,enabled,revision,confirmed_at,not_before FROM auto_sign_tasks`
	var args []any
	if account != "" {
		query += " WHERE account=?"
		args = append(args, account)
	}
	query += " ORDER BY id DESC"
	rows, e := s.db.Query(query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	tasks := []Task{}
	for rows.Next() {
		var t Task
		var config, confirmed, before string
		if e = rows.Scan(&t.ID, &t.Account, &config, &t.Enabled, &t.Revision, &confirmed, &before); e != nil {
			return nil, e
		}
		var content Task
		if e = json.Unmarshal([]byte(config), &content); e != nil {
			return nil, e
		}
		content.ID = t.ID
		content.Account = t.Account
		content.Enabled = t.Enabled
		content.Revision = t.Revision
		content.LastRun = nil
		content.ConfirmedAt, e = time.Parse(time.RFC3339Nano, confirmed)
		if e != nil {
			return nil, e
		}
		content.NotBefore, e = time.Parse(time.RFC3339Nano, before)
		if e != nil {
			return nil, e
		}
		tasks = append(tasks, content)
	}
	return tasks, rows.Err()
}
func (s *Store) Task(account string, id int64) (Task, error) {
	tasks, e := s.Tasks(account)
	if e != nil {
		return Task{}, e
	}
	for _, t := range tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return Task{}, ErrNotFound
}
func (s *Store) SetEnabled(account string, id, revision int64, enabled bool, now time.Time) (Task, error) {
	t, e := s.Task(account, id)
	if e != nil {
		return Task{}, e
	}
	if t.Revision != revision {
		return Task{}, ErrConflict
	}
	result, e := s.db.Exec(`UPDATE auto_sign_tasks SET enabled=?,revision=revision+1,not_before=?,confirmed_at=CASE WHEN ? THEN ? ELSE confirmed_at END WHERE id=? AND account=? AND revision=?`, enabled, stamp(now), enabled, stamp(now), id, account, revision)
	if e != nil {
		return Task{}, e
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return Task{}, ErrConflict
	}
	return s.Task(account, id)
}
func (s *Store) Delete(account string, id, revision int64) error {
	r, e := s.db.Exec("DELETE FROM auto_sign_tasks WHERE id=? AND account=? AND revision=?", id, account, revision)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return ErrConflict
	}
	return nil
}
func (s *Store) Runs(account string) ([]Run, error) {
	rows, e := s.db.Query(`SELECT id,task_id,account,name,revision,date,scheduled_time,started_at,finished_at,status,message,stage,failure_kind,verification_attempts FROM auto_sign_runs WHERE account=? ORDER BY id DESC LIMIT 100`, account)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	runs := []Run{}
	for rows.Next() {
		var r Run
		if e = rows.Scan(&r.ID, &r.TaskID, &r.Account, &r.Name, &r.Revision, &r.Date, &r.ScheduledTime, &r.StartedAt, &r.FinishedAt, &r.Status, &r.Message, &r.Stage, &r.FailureKind, &r.VerificationAttempts); e != nil {
			return nil, e
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}
func (s *Store) claim(t Task, now time.Time) (bool, error) {
	return s.claimAt(t, now, now)
}
func (s *Store) claimAt(t Task, now, actual time.Time) (bool, error) {
	date := now.In(Shanghai).Format("2006-01-02")
	r, e := s.db.Exec(`INSERT INTO auto_sign_runs(task_id,account,name,revision,date,scheduled_time,started_at,status)
 SELECT id,account,?,revision,?,?,?,'running' FROM auto_sign_tasks
 WHERE id=? AND account=? AND revision=? AND enabled=1 AND NOT EXISTS(SELECT 1 FROM auto_sign_runs WHERE account=? AND status='running')
 ON CONFLICT(task_id,date) DO NOTHING`, t.Name, date, t.Time, stamp(actual), t.ID, t.Account, t.Revision, t.Account)
	if e != nil {
		return false, e
	}
	n, e := r.RowsAffected()
	return n == 1, e
}

// finishRun persists the final status and, for a final failure, creates at most
// one notification event in the same transaction. Submission may have happened,
// so pending_verification is not notified until read-only verification ends.
func (s *Store) finish(t Task, now time.Time, status, message, stage, failureKind string) error {
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	date := now.In(Shanghai).Format("2006-01-02")
	var runID int64
	e = tx.QueryRow(`SELECT id FROM auto_sign_runs WHERE task_id=? AND date=? AND status='running'`, t.ID, date).Scan(&runID)
	if errors.Is(e, sql.ErrNoRows) {
		return tx.Commit()
	}
	if e != nil {
		return e
	}
	nextVerify := ""
	if status == "pending_verification" {
		nextVerify = stamp(now.Add(time.Minute))
	}
	result, e := tx.Exec(`UPDATE auto_sign_runs SET status=?,message=?,stage=?,failure_kind=?,finished_at=?,verification_done=0,next_verify_at=? WHERE id=? AND status='running'`, status, message, stage, failureKind, stamp(now), nextVerify, runID)
	if e != nil {
		return e
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return tx.Commit()
	}
	if status == "failed" {
		if e = queueNotification(tx, runID, now); e != nil {
			return e
		}
	}
	return tx.Commit()
}

func queueNotification(tx *sql.Tx, runID int64, now time.Time) error {
	_, e := tx.Exec(`INSERT OR IGNORE INTO auto_sign_notifications(run_id,task_id,account,name,date,scheduled_time,result,user_id,email,email_version,state,reason,attempts,last_error,created_at,next_attempt_at)
 SELECT r.id,r.task_id,r.account,r.name,r.date,r.scheduled_time,r.status,COALESCE(b.user_id,0),COALESCE(n.email,''),COALESCE(n.version,0),
 CASE WHEN b.user_id IS NULL THEN 'skipped' WHEN n.user_id IS NULL THEN 'skipped' WHEN n.enabled=0 THEN 'skipped' ELSE 'pending' END,
 CASE WHEN b.user_id IS NULL THEN 'no_user' WHEN n.user_id IS NULL THEN 'no_email' WHEN n.enabled=0 THEN 'disabled' ELSE '' END,
 0,'',?,CASE WHEN b.user_id IS NOT NULL AND n.user_id IS NOT NULL AND n.enabled=1 THEN ? ELSE '' END
 FROM auto_sign_runs r
 LEFT JOIN platform_bindings b ON b.account=r.account
 LEFT JOIN platform_notification_emails n ON n.user_id=b.user_id
 WHERE r.id=?`, stamp(now), stamp(now), runID)
	return e
}

// recover marks interrupted runs. Only an interruption during the submit stage
// is ambiguous; earlier stages are known not to have submitted.
func (s *Store) recover(now time.Time) error {
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	rows, e := tx.Query(`SELECT id,stage FROM auto_sign_runs WHERE status='running' AND started_at<?`, stamp(now.Add(-2*time.Minute)))
	if e != nil {
		return e
	}
	type interrupted struct {
		id        int64
		ambiguous bool
	}
	list := []interrupted{}
	for rows.Next() {
		var id int64
		var stage string
		if e = rows.Scan(&id, &stage); e != nil {
			rows.Close()
			return e
		}
		list = append(list, interrupted{id: id, ambiguous: stage == "sign"})
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	failed := 0
	for _, item := range list {
		status, message, kind := "failed", "上次执行中断，本次未提交签到；请查看官方签到记录", "interrupted"
		next := ""
		if item.ambiguous {
			status = "pending_verification"
			message = "执行中断，无法确认是否已提交；请先查看官方签到记录，系统不会自动重试"
			next = stamp(now.Add(time.Minute))
		}
		if _, e = tx.Exec(`UPDATE auto_sign_runs SET status=?,message=?,stage=COALESCE(NULLIF(stage,''),'interrupted'),failure_kind=?,finished_at=?,verification_done=0,next_verify_at=? WHERE id=? AND status='running'`, status, message, kind, stamp(now), next, item.id); e != nil {
			return e
		}
		if status == "failed" {
			failed++
			if e = queueNotification(tx, item.id, now); e != nil {
				return e
			}
		}
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	if len(list) > 0 {
		log.Printf("[autosign] stage=recovery status=processed interrupted_runs=%d failed=%d", len(list), failed)
	}
	return nil
}

// recordMissed persists planned executions that did not start while the task
// was enabled. It never submits and never sends email by default.
func (s *Store) recordMissed(now time.Time) error {
	tasks, e := s.Tasks("")
	if e != nil {
		return e
	}
	today := now.In(Shanghai).Format("2006-01-02")
	created := 0
	for _, t := range tasks {
		if !t.Enabled {
			continue
		}
		due, e := time.ParseInLocation("2006-01-02 15:04", today+" "+t.Time, Shanghai)
		if e != nil || now.Before(due.Add(2*time.Minute)) || due.Before(t.NotBefore) {
			continue
		}
		result, e := s.db.Exec(`INSERT OR IGNORE INTO auto_sign_runs(task_id,account,name,revision,date,scheduled_time,started_at,finished_at,status,message,stage,failure_kind)
 VALUES(?,?,?,?,?,?,?,?, 'missed','服务在计划时间未执行（可能停机或调度未运行），本次已错过；不会自动补签','scheduler','missed_window')`, t.ID, t.Account, t.Name, t.Revision, today, t.Time, stamp(now), stamp(now))
		if e != nil {
			return e
		}
		if n, _ := result.RowsAffected(); n > 0 {
			created++
		}
	}
	if created > 0 {
		log.Printf("[autosign] stage=scheduler status=missed missed_runs=%d date=%s", created, today)
	}
	return nil
}
func (s *Store) SavePrecheck(taskID int64, date, status, message string, attempts int, now, next time.Time) error {
	nextAt := ""
	if !next.IsZero() {
		nextAt = stamp(next)
	}
	_, e := s.db.Exec(`INSERT INTO auto_sign_prechecks(task_id,date,status,message,attempts,checked_at,next_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(task_id,date) DO UPDATE SET status=excluded.status,message=excluded.message,attempts=excluded.attempts,checked_at=excluded.checked_at,next_at=excluded.next_at`, taskID, date, status, message, attempts, stamp(now), nextAt)
	return e
}

func (s *Store) Precheck(taskID int64, date string) (Precheck, bool, error) {
	var p Precheck
	p.TaskID, p.Date = taskID, date
	e := s.db.QueryRow(`SELECT status,message,attempts,checked_at,next_at FROM auto_sign_prechecks WHERE task_id=? AND date=?`, taskID, date).Scan(&p.Status, &p.Message, &p.Attempts, &p.CheckedAt, &p.NextAt)
	if errors.Is(e, sql.ErrNoRows) {
		return p, false, nil
	}
	return p, e == nil, e
}

func (s *Store) Prechecks(date string) (map[int64]Precheck, error) {
	rows, e := s.db.Query(`SELECT task_id,date,status,message,attempts,checked_at,next_at FROM auto_sign_prechecks WHERE date=?`, date)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	result := map[int64]Precheck{}
	for rows.Next() {
		var p Precheck
		if e = rows.Scan(&p.TaskID, &p.Date, &p.Status, &p.Message, &p.Attempts, &p.CheckedAt, &p.NextAt); e != nil {
			return nil, e
		}
		result[p.TaskID] = p
	}
	return result, rows.Err()
}

func (s *Store) PendingVerifications(now time.Time) ([]Run, error) {
	rows, e := s.db.Query(`SELECT id,task_id,account,name,date,scheduled_time,verification_attempts,message FROM auto_sign_runs WHERE status='pending_verification' AND verification_done=0 AND next_verify_at<>'' AND next_verify_at<=? ORDER BY id`, stamp(now))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	runs := []Run{}
	for rows.Next() {
		var r Run
		if e = rows.Scan(&r.ID, &r.TaskID, &r.Account, &r.Name, &r.Date, &r.ScheduledTime, &r.VerificationAttempts, &r.Message); e != nil {
			return nil, e
		}
		r.Status = "pending_verification"
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// resolveVerification only changes a still-pending run. A later success or
// already-signed result must never be overwritten by a stale verification.
func (s *Store) resolveVerification(runID int64, found bool, now time.Time) error {
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var status, message string
	var attempts int
	e = tx.QueryRow(`SELECT status,message,verification_attempts FROM auto_sign_runs WHERE id=?`, runID).Scan(&status, &message, &attempts)
	if errors.Is(e, sql.ErrNoRows) || (e == nil && status != "pending_verification") {
		return tx.Commit()
	}
	if e != nil {
		return e
	}
	if found {
		_, e = tx.Exec(`UPDATE auto_sign_runs SET status='already_signed',message=? ,stage='verify',failure_kind='',verification_done=1,next_verify_at='' WHERE id=? AND status='pending_verification'`, "只读复核发现官方当日签到记录，已更新为已签到；未重复提交", runID)
	} else {
		attempts++
		if attempts >= 3 {
			message += "；已连续复核仍无法确认，请查看官方签到记录"
			if _, e = tx.Exec(`UPDATE auto_sign_runs SET message=?,verification_attempts=?,verification_done=1,next_verify_at='' WHERE id=? AND status='pending_verification'`, message, attempts, runID); e != nil {
				return e
			}
			if e = queueNotification(tx, runID, now); e != nil {
				return e
			}
		} else {
			if _, e = tx.Exec(`UPDATE auto_sign_runs SET verification_attempts=?,next_verify_at=? WHERE id=? AND status='pending_verification'`, attempts, stamp(now.Add(2*time.Minute)), runID); e != nil {
				return e
			}
		}
	}
	if e != nil {
		return e
	}
	return tx.Commit()
}

func (s *Store) CheckSchool(t Task) error {
	id, e := s.School(t.Account)
	if e != nil {
		return e
	}
	if id != t.SchoolID {
		return fmt.Errorf("学校信息已变化，请编辑并重新确认任务")
	}
	return nil
}
