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
}
type Run struct {
	ID            int64  `json:"id"`
	TaskID        int64  `json:"task_id"`
	Account       string `json:"account"`
	Name          string `json:"name"`
	Revision      int64  `json:"revision"`
	Date          string `json:"date"`
	ScheduledTime string `json:"scheduled_time"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
	Status        string `json:"status"`
	Message       string `json:"message"`
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
 finished_at TEXT NOT NULL DEFAULT '',status TEXT NOT NULL,message TEXT NOT NULL DEFAULT '',UNIQUE(task_id,date));
 CREATE INDEX IF NOT EXISTS auto_sign_runs_account ON auto_sign_runs(account,id);
 CREATE INDEX IF NOT EXISTS auto_sign_runs_date ON auto_sign_runs(date,status);
 CREATE INDEX IF NOT EXISTS auto_sign_runs_task ON auto_sign_runs(task_id,id);`)
	if err != nil {
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
	rows, e := s.db.Query(`SELECT id,task_id,account,name,revision,date,scheduled_time,started_at,finished_at,status,message FROM auto_sign_runs WHERE account=? ORDER BY id DESC LIMIT 100`, account)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	runs := []Run{}
	for rows.Next() {
		var r Run
		if e = rows.Scan(&r.ID, &r.TaskID, &r.Account, &r.Name, &r.Revision, &r.Date, &r.ScheduledTime, &r.StartedAt, &r.FinishedAt, &r.Status, &r.Message); e != nil {
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
func (s *Store) finish(t Task, now time.Time, status, message string) error {
	_, e := s.db.Exec(`UPDATE auto_sign_runs SET status=?,message=?,finished_at=? WHERE task_id=? AND date=? AND status='running'`, status, message, stamp(time.Now()), t.ID, now.In(Shanghai).Format("2006-01-02"))
	return e
}
func (s *Store) recover(now time.Time) error {
	result, e := s.db.Exec(`UPDATE auto_sign_runs SET status='failed',message='上次执行中断，未确认成功；可能已提交，请先查看官方签到记录，系统不会自动重试',finished_at=? WHERE status='running' AND started_at<?`, stamp(now), stamp(now.Add(-2*time.Minute)))
	if e == nil {
		if count, _ := result.RowsAffected(); count > 0 {
			log.Printf("[autosign] stage=recovery status=failed interrupted_runs=%d", count)
		}
	}
	return e
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
