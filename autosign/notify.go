package autosign

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// MailOutcome distinguishes a definite send from an ambiguous or retryable failure.
// A message is never retried after SMTP accepted the DATA payload.
type MailOutcome string

const (
	MailSent      MailOutcome = "sent"
	MailRetryable MailOutcome = "retryable"
	MailUncertain MailOutcome = "uncertain"
)

// MailSender is the minimal SMTP surface needed by background notifications.
type MailSender interface {
	SendMessage(ctx context.Context, to, subject, body string) (MailOutcome, error)
}

type Notifier struct {
	store  *Store
	sender MailSender
	now    func() time.Time
	mu     sync.Mutex
}

type notification struct {
	ID            int64
	RunID         int64
	TaskID        int64
	UserID        int64
	Account       string
	Name          string
	Date          string
	ScheduledTime string
	Result        string
	Email         string
	EmailVersion  int64
	Attempts      int
	State         string
}

func NewNotifier(store *Store, sender MailSender) *Notifier {
	return &Notifier{store: store, sender: sender, now: time.Now}
}

func (n *Notifier) Run(ctx context.Context) {
	// A crash can leave an event in "sending". Only one notifier runs per
	// deployment, so returning it to pending on startup is safe.
	n.store.resetSendingNotifications()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	n.Process(ctx, n.now())
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.Process(ctx, n.now())
		}
	}
}

func (n *Notifier) Process(ctx context.Context, now time.Time) {
	if n.store == nil || n.sender == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	events, e := n.store.DueNotifications(now)
	if e != nil {
		log.Printf("[notify] stage=collect status=failed error_class=%T", e)
		return
	}
	for _, event := range events {
		if ctx.Err() != nil {
			return
		}
		n.deliver(ctx, event, now)
	}
}

func (n *Notifier) deliver(ctx context.Context, event notification, now time.Time) {
	claimed, e := n.store.claimNotification(event.ID)
	if e != nil || !claimed {
		return
	}
	status, verificationDone, e := n.store.NotificationRunState(event.RunID)
	if errors.Is(e, sql.ErrNoRows) {
		n.store.closeNotification(event.ID, "cancelled", "result_changed", now)
		return
	}
	if e == nil && (status != "failed" && status != "pending_verification" || status == "pending_verification" && verificationDone == 0) {
		n.store.closeNotification(event.ID, "cancelled", "result_changed", now)
		return
	}
	if e != nil {
		n.store.retryNotification(event.ID, event.Attempts, "database_error", now.Add(time.Minute))
		return
	}
	current, bound, e := n.store.CurrentNotificationEmail(event.UserID)
	if e != nil {
		n.store.retryNotification(event.ID, event.Attempts, "database_error", now.Add(time.Minute))
		return
	}
	if !bound || !current.Enabled || current.Email != event.Email || current.Version != event.EmailVersion {
		n.store.closeNotification(event.ID, "cancelled", "email_changed", now)
		return
	}
	subject, body := notificationMessage(event)
	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	outcome, err := n.sender.SendMessage(sendCtx, event.Email, subject, body)
	cancel()
	switch outcome {
	case MailSent:
		n.store.closeNotification(event.ID, "sent", "", now)
		log.Printf("[notify] run_id=%d stage=send status=sent attempts=%d", event.RunID, event.Attempts+1)
	case MailUncertain:
		n.store.closeNotification(event.ID, "uncertain", "delivery_uncertain", now)
		log.Printf("[notify] run_id=%d stage=send status=uncertain attempts=%d error_class=%T", event.RunID, event.Attempts+1, err)
	default:
		attempts := event.Attempts + 1
		if attempts >= 3 {
			n.store.closeNotification(event.ID, "failed", "smtp_error", now)
			log.Printf("[notify] run_id=%d stage=send status=failed attempts=%d error_class=%T", event.RunID, attempts, err)
			return
		}
		delay := time.Minute
		if attempts == 2 {
			delay = 5 * time.Minute
		}
		n.store.retryNotification(event.ID, attempts, "smtp_error", now.Add(delay))
		log.Printf("[notify] run_id=%d stage=send status=retry attempts=%d error_class=%T", event.RunID, attempts, err)
	}
}

func notificationMessage(event notification) (string, string) {
	subject := fmt.Sprintf("【讯签到】%s 签到结果提醒", event.Date)
	status := "失败"
	advice := "请前往习讯云查看官方签到记录。系统不会自动重试本次签到。"
	if event.Result == "pending_verification" {
		status = "待核对"
		advice = "系统已进行只读复核，但仍无法确认结果。请先查看官方签到记录；如未成功可手动签到。系统不会自动重试。"
	}
	body := fmt.Sprintf("任务：%s\r\n日期：%s\r\n计划时间：%s\r\n结果：%s\r\n说明：%s\r\n\r\n%s\r\n\r\n如需关闭或更换提醒邮箱，请登录讯签到账户设置页面。本邮件由系统自动发送，请勿直接回复。\r\n", event.Name, event.Date, event.ScheduledTime, status,
		func() string {
			switch event.Result {
			case "pending_verification":
				return "提交后未能确认签到结果；如需可通过官方记录核对是否成功。"
			case "failed":
				return "本次签到最终失败，未确认成功。"
			}
			return "本次签到结果异常。"
		}(), advice)
	return subject, body
}

func (s *Store) DueNotifications(now time.Time) ([]notification, error) {
	rows, e := s.db.Query(`SELECT id,run_id,task_id,user_id,account,name,date,scheduled_time,result,email,email_version,attempts,state FROM auto_sign_notifications WHERE state='pending' AND next_attempt_at<>'' AND next_attempt_at<=? ORDER BY id LIMIT 50`, stamp(now))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	events := []notification{}
	for rows.Next() {
		var n notification
		if e = rows.Scan(&n.ID, &n.RunID, &n.TaskID, &n.UserID, &n.Account, &n.Name, &n.Date, &n.ScheduledTime, &n.Result, &n.Email, &n.EmailVersion, &n.Attempts, &n.State); e != nil {
			return nil, e
		}
		events = append(events, n)
	}
	return events, rows.Err()
}

func (s *Store) resetSendingNotifications() error {
	_, e := s.db.Exec(`UPDATE auto_sign_notifications SET state='pending' WHERE state='sending'`)
	return e
}

func (s *Store) claimNotification(id int64) (bool, error) {
	result, e := s.db.Exec(`UPDATE auto_sign_notifications SET state='sending' WHERE id=? AND state='pending'`, id)
	if e != nil {
		return false, e
	}
	count, e := result.RowsAffected()
	return count == 1, e
}

func (s *Store) closeNotification(id int64, state, reason string, now time.Time) error {
	_, e := s.db.Exec(`UPDATE auto_sign_notifications SET state=?,reason=?,last_error=?,next_attempt_at='',sent_at=CASE WHEN ?='sent' THEN ? ELSE sent_at END WHERE id=?`, state, reason, reason, state, stamp(now), id)
	return e
}

func (s *Store) retryNotification(id int64, attempts int, reason string, next time.Time) error {
	_, e := s.db.Exec(`UPDATE auto_sign_notifications SET state='pending',attempts=?,last_error=?,next_attempt_at=? WHERE id=?`, attempts, reason, stamp(next), id)
	return e
}

func (s *Store) NotificationRunState(runID int64) (string, int, error) {
	var status string
	var done int
	e := s.db.QueryRow(`SELECT status,verification_done FROM auto_sign_runs WHERE id=?`, runID).Scan(&status, &done)
	return status, done, e
}

type NotificationEmail struct {
	Email   string `json:"email"`
	Enabled bool   `json:"enabled"`
	Version int64  `json:"version"`
}

func (s *Store) CurrentNotificationEmail(userID int64) (NotificationEmail, bool, error) {
	var n NotificationEmail
	var enabled int
	e := s.db.QueryRow(`SELECT email,enabled,version FROM platform_notification_emails WHERE user_id=?`, userID).Scan(&n.Email, &enabled, &n.Version)
	if errors.Is(e, sql.ErrNoRows) {
		return n, false, nil
	}
	n.Enabled = enabled == 1
	return n, e == nil, e
}
