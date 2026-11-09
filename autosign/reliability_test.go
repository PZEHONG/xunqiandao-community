package autosign

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"xixunyunsign/service"
)

type submitSign struct {
	service.SignService
	calls      int
	remarksErr error
	err        error
}

func (f *submitSign) Remarks(context.Context, string) ([]service.SignRemark, error) {
	if f.remarksErr != nil {
		return nil, f.remarksErr
	}
	return []service.SignRemark{{Key: "0", Value: "上班"}}, nil
}

func (f *submitSign) SignInWithOptions(_ context.Context, _, _, _, _, _ string, o service.SignOptions) (string, error) {
	if o.BeforeSubmit != nil {
		if e := o.BeforeSubmit(); e != nil {
			return "", e
		}
	}
	f.calls++
	if o.OnSubmitting != nil {
		o.OnSubmitting()
	}
	return "", f.err
}

func TestAmbiguousSubmitIsVerifiedReadOnly(t *testing.T) {
	s := testStore(t)
	start := at("2026-10-15T00:59:58Z")
	save(t, s, start)
	f := &submitSign{err: &service.SignFailure{Kind: "timeout", Message: "请求超时"}}
	r := NewRunner(s, f, start, recordCheck(func(context.Context, string, time.Time) (bool, error) { return false, nil }))
	r.Tick(context.Background(), start.Add(3*time.Second))
	runs, e := s.Runs("u")
	if e != nil || len(runs) != 1 {
		t.Fatalf("ambiguous run missing: %#v %v", runs, e)
	}
	if runs[0].Status != "pending_verification" || runs[0].Stage != "sign" || runs[0].FailureKind != "timeout" || !strings.Contains(runs[0].Message, "不会自动重试") {
		t.Fatalf("ambiguous submit should wait for verification: %#v", runs[0])
	}
	if f.calls != 1 {
		t.Fatal("submission was repeated")
	}
	r.checker = recordCheck(func(context.Context, string, time.Time) (bool, error) { return true, nil })
	r.Verify(context.Background(), start.Add(2*time.Second).Add(time.Minute+time.Second))
	runs, _ = s.Runs("u")
	if len(runs) != 1 || runs[0].Status != "already_signed" || f.calls != 1 {
		t.Fatalf("official record not applied or resubmitted: %#v calls=%d", runs, f.calls)
	}
}

func TestUnconfirmedVerificationStopsAfterThreeAttempts(t *testing.T) {
	s := testStore(t)
	start := at("2026-10-15T00:59:58Z")
	save(t, s, start)
	f := &submitSign{err: &service.SignFailure{Kind: "transport_error", Message: "连接中断"}}
	r := NewRunner(s, f, start, recordCheck(func(context.Context, string, time.Time) (bool, error) { return false, nil }))
	r.Tick(context.Background(), start.Add(3*time.Second))
	due := start.Add(2 * time.Second)
	for i, offset := range []time.Duration{61 * time.Second, 182 * time.Second, 303 * time.Second} {
		r.Verify(context.Background(), due.Add(offset))
		runs, _ := s.Runs("u")
		if runs[0].Status != "pending_verification" || runs[0].VerificationAttempts != i+1 {
			t.Fatalf("attempt %d not persisted: %#v", i+1, runs[0])
		}
	}
	runs, _ := s.Runs("u")
	if runs[0].Status != "pending_verification" || runs[0].VerificationAttempts != 3 || f.calls != 1 {
		t.Fatalf("verification should end unconfirmed without retry: %#v calls=%d", runs[0], f.calls)
	}
	var done int
	s.db.QueryRow(`SELECT verification_done FROM auto_sign_runs WHERE task_id=? AND date=?`, runs[0].TaskID, runs[0].Date).Scan(&done)
	if done != 1 {
		t.Fatal("verification not marked done")
	}
	var events int
	s.db.QueryRow(`SELECT COUNT(*) FROM auto_sign_notifications WHERE run_id=?`, runs[0].ID).Scan(&events)
	if events != 1 {
		t.Fatalf("final ambiguous result must create one notification event, got %d", events)
	}
}

func TestExplicitFailureCreatesExactlyOneNotification(t *testing.T) {
	for _, c := range []struct{ code, kind string }{{"99999", "upstream_99999"}, {"64033", "interval_64033"}} {
		t.Run(c.code, func(t *testing.T) {
			s := testStore(t)
			start := at("2026-10-15T00:59:58Z")
			save(t, s, start)
			f := &submitSign{err: &service.SignFailure{Kind: "upstream_rejected", Code: c.code, Message: "平台返回签到失败"}}
			r := NewRunner(s, f, start, recordCheck(func(context.Context, string, time.Time) (bool, error) { return false, nil }))
			r.Tick(context.Background(), start.Add(3*time.Second))
			runs, _ := s.Runs("u")
			if len(runs) != 1 || runs[0].Status != "failed" || runs[0].FailureKind != c.kind || !strings.Contains(runs[0].Message, c.code) {
				t.Fatalf("explicit rejection misclassified: %#v", runs)
			}
			var events int
			s.db.QueryRow(`SELECT COUNT(*) FROM auto_sign_notifications WHERE run_id=?`, runs[0].ID).Scan(&events)
			if events != 1 {
				t.Fatalf("exactly one event expected, got %d", events)
			}
		})
	}
}

func TestPrecheckRunsBeforePlannedTimeWithoutSubmitting(t *testing.T) {
	s := testStore(t)
	start := at("2026-10-15T08:44:58+08:00")
	save(t, s, start)
	f := &submitSign{}
	r := NewRunner(s, f, start, recordCheck(func(context.Context, string, time.Time) (bool, error) { return false, nil }))
	r.Tick(context.Background(), start.Add(5*time.Second))
	if f.calls != 0 {
		t.Fatal("precheck must never submit")
	}
	runs, _ := s.Runs("u")
	if len(runs) != 0 {
		t.Fatalf("precheck created a run: %#v", runs)
	}
	tasks, _ := s.Tasks("u")
	p, found, e := s.Precheck(tasks[0].ID, "2026-10-15")
	if e != nil || !found || p.Status != "ready" || p.Attempts != 1 {
		t.Fatalf("precheck not persisted: %#v found=%t err=%v", p, found, e)
	}
}

type fakeMailSender struct {
	outcome MailOutcome
	err     error
	sent    []string
}

func (f *fakeMailSender) SendMessage(context.Context, string, string, string) (MailOutcome, error) {
	if f.outcome == MailSent {
		f.sent = append(f.sent, "sent")
	}
	return f.outcome, f.err
}

func seedNotification(t *testing.T, s *Store, date, result string) int64 {
	t.Helper()
	var runID int64
	e := s.db.QueryRow(`INSERT INTO auto_sign_runs(task_id,account,name,revision,date,scheduled_time,started_at,finished_at,status,message,failure_kind,verification_done) VALUES(1,'u','上班',1,?, '09:00','','',?,'', 'upstream_99999',1) RETURNING id`, date, result).Scan(&runID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.db.Exec(`INSERT INTO platform_notification_emails(user_id,email,enabled,version,verified_at,updated_at) VALUES(1,?,1,1,?,?) ON CONFLICT(user_id) DO UPDATE SET email=excluded.email,enabled=1,version=1`, "notify@example.com", time.Now().Unix(), time.Now().Unix())
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.db.Exec(`INSERT INTO auto_sign_notifications(run_id,task_id,account,name,date,scheduled_time,result,user_id,email,email_version,state,reason,attempts,last_error,created_at,next_attempt_at) VALUES(?,1,'u','上班',?, '09:00',?,1,'notify@example.com',1,'pending','',0,'',? ,?)`, runID, date, result, stamp(time.Now()), stamp(time.Now()))
	if e != nil {
		t.Fatal(e)
	}
	return runID
}

func TestNotificationSenderClassifiesOutcomes(t *testing.T) {
	s := testStore(t)
	seedNotification(t, s, "2026-10-15", "failed")
	sender := &fakeMailSender{outcome: MailSent}
	n := NewNotifier(s, sender)
	n.Process(context.Background(), time.Now().Add(time.Second))
	var state string
	s.db.QueryRow(`SELECT state FROM auto_sign_notifications WHERE run_id=(SELECT MAX(run_id) FROM auto_sign_notifications)`).Scan(&state)
	if state != "sent" || len(sender.sent) != 1 {
		t.Fatalf("sent event wrong: %q sent=%d", state, len(sender.sent))
	}

	seedNotification(t, s, "2026-10-16", "failed")
	sender = &fakeMailSender{outcome: MailUncertain, err: errors.New("connection lost")}
	NewNotifier(s, sender).Process(context.Background(), time.Now().Add(time.Second))
	s.db.QueryRow(`SELECT state FROM auto_sign_notifications WHERE run_id=(SELECT MAX(run_id) FROM auto_sign_notifications)`).Scan(&state)
	if state != "uncertain" {
		t.Fatalf("ambiguous delivery must not retry: %q", state)
	}
}

func TestNotificationIsCancelledWhenEmailChanges(t *testing.T) {
	s := testStore(t)
	runID := seedNotification(t, s, "2026-10-15", "failed")
	s.db.Exec(`UPDATE platform_notification_emails SET version=2,email='new@example.com' WHERE user_id=1`)
	sender := &fakeMailSender{outcome: MailSent}
	NewNotifier(s, sender).Process(context.Background(), time.Now().Add(time.Second))
	var state string
	s.db.QueryRow(`SELECT state FROM auto_sign_notifications WHERE run_id=?`, runID).Scan(&state)
	if state != "cancelled" || len(sender.sent) != 0 {
		t.Fatalf("stale email event not cancelled: %q sent=%d", state, len(sender.sent))
	}
}

func TestNotificationRetriesOnlyRetryableFailure(t *testing.T) {
	s := testStore(t)
	runID := seedNotification(t, s, "2026-10-15", "failed")
	s.db.Exec(`UPDATE auto_sign_notifications SET next_attempt_at=? WHERE run_id=?`, stamp(time.Now().Add(-time.Second)), runID)
	sender := &fakeMailSender{outcome: MailRetryable, err: errors.New("smtp unavailable")}
	n := NewNotifier(s, sender)
	n.Process(context.Background(), time.Now())
	var attempts int
	var state string
	s.db.QueryRow(`SELECT attempts,state FROM auto_sign_notifications WHERE run_id=?`, runID).Scan(&attempts, &state)
	if attempts != 1 || state != "pending" {
		t.Fatalf("retryable failure not queued: attempts=%d state=%q", attempts, state)
	}
}
