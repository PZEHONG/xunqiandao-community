package autosign

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"xixunyunsign/service"
)

type recordCheck func(context.Context, string, time.Time) (bool, error)

func (f recordCheck) HasSignInOn(ctx context.Context, account string, day time.Time) (bool, error) {
	return f(ctx, account, day)
}

func TestDailyGuardSkipsAnyOfficialRecordAndFailsClosed(t *testing.T) {
	for _, mode := range []string{"manual", "other-type", "abnormal", "query-error", "missing-checker", "empty"} {
		t.Run(mode, func(t *testing.T) {
			s := testStore(t)
			start := at("2026-10-25T16:59:58Z") // 26 October in Shanghai.
			task := sample()
			task.Time = "01:00"
			if _, err := s.Save(task, start); err != nil {
				t.Fatal(err)
			}
			sign := &fakeSign{}
			var checker service.DailySignChecker = recordCheck(func(ctx context.Context, account string, day time.Time) (bool, error) {
				if account != "u" || day.In(Shanghai).Format("2006-01-02") != "2026-10-26" {
					t.Fatal("wrong account or Shanghai day")
				}
				if mode == "query-error" {
					return false, errors.New("private-token account-address")
				}
				return mode != "empty", nil
			})
			if mode == "missing-checker" {
				checker = nil
			}
			NewRunner(s, sign, start, checker).Tick(context.Background(), start.Add(3*time.Second))
			runs, err := s.Runs("u")
			want := "already_signed"
			if mode == "query-error" || mode == "missing-checker" {
				want = "failed"
			}
			if mode == "empty" {
				want = "success"
			}
			if err != nil || len(runs) != 1 || runs[0].Status != want {
				t.Fatalf("runs=%#v error=%v", runs, err)
			}
			if (sign.calls == 1) != (mode == "empty") {
				t.Fatalf("unexpected actual submission count %d", sign.calls)
			}
			if strings.Contains(runs[0].Message, "private-token") {
				t.Fatal("private error leaked")
			}
			if want == "already_signed" {
				if _, err := NewStore(s.db); err != nil {
					t.Fatal(err)
				}
				runs, _ = s.Runs("u")
				if runs[0].Status != "already_signed" {
					t.Fatal("restart lost skip result")
				}
			}
		})
	}
}

func TestDailyGuardRetainsSuccessAcrossDeleteAndRecreate(t *testing.T) {
	for _, mode := range []string{"today-success", "today-failed", "yesterday-success", "other-account"} {
		t.Run(mode, func(t *testing.T) {
			s := testStore(t)
			start := at("2026-10-26T08:59:58+08:00")
			old := save(t, s, start.Add(-time.Hour))
			date := start
			if mode == "yesterday-success" {
				date = date.AddDate(0, 0, -1)
			}
			if claimed, err := s.claim(old, date); err != nil || !claimed {
				t.Fatal(claimed, err)
			}
			status := "success"
			if mode == "today-failed" {
				status = "failed"
			}
			if err := s.finish(old, date, status, "old run", "", ""); err != nil {
				t.Fatal(err)
			}
			if err := s.Delete("u", old.ID, old.Revision); err != nil {
				t.Fatal(err)
			}
			task := sample()
			if mode == "other-account" {
				task.Account = "v"
				task.SchoolID, _ = s.School("v")
			}
			if _, err := s.Save(task, start); err != nil {
				t.Fatal(err)
			}
			sign := &fakeSign{}
			checker := recordCheck(func(context.Context, string, time.Time) (bool, error) { return false, nil })
			NewRunner(s, sign, start, checker).Tick(context.Background(), start.Add(3*time.Second))
			wantCalls := 1
			if mode == "today-success" {
				wantCalls = 0
			}
			if sign.calls != wantCalls {
				t.Fatalf("submissions=%d want=%d", sign.calls, wantCalls)
			}
		})
	}
}

type guardedSubmitSign struct {
	fakeSign
	before func()
}

func (s *guardedSubmitSign) SignInWithOptions(ctx context.Context, a, b, c, d, e string, o service.SignOptions) (string, error) {
	if s.before != nil {
		s.before()
	}
	if err := o.BeforeSubmit(); err != nil {
		return "", err
	}
	return s.fakeSign.SignInWithOptions(ctx, a, b, c, d, e, o)
}

func TestDailyGuardRechecksImmediatelyBeforeSubmission(t *testing.T) {
	for _, mode := range []string{"manual-sign-during-preparation", "query-fails-at-submit", "paused-during-final-check"} {
		t.Run(mode, func(t *testing.T) {
			s := testStore(t)
			start := at("2026-10-26T08:59:58+08:00")
			task := save(t, s, start)
			checks := 0
			checker := recordCheck(func(context.Context, string, time.Time) (bool, error) {
				checks++
				if checks == 1 {
					return false, nil
				}
				switch mode {
				case "manual-sign-during-preparation":
					return true, nil
				case "query-fails-at-submit":
					return false, errors.New("private-token")
				default:
					if _, err := s.SetEnabled("u", task.ID, task.Revision, false, start.Add(time.Second)); err != nil {
						t.Fatal(err)
					}
					return false, nil
				}
			})
			sign := &guardedSubmitSign{}
			NewRunner(s, sign, start, checker).Tick(context.Background(), start.Add(3*time.Second))
			runs, _ := s.Runs("u")
			want := "failed"
			if mode == "manual-sign-during-preparation" {
				want = "already_signed"
			}
			if mode == "paused-during-final-check" {
				want = "cancelled"
			}
			if sign.calls != 0 || len(runs) != 1 || runs[0].Status != want {
				t.Fatalf("submissions=%d runs=%#v", sign.calls, runs)
			}
		})
	}
}

func TestDailyGuardStopsIfPreparationCrossesShanghaiMidnight(t *testing.T) {
	s := testStore(t)
	actual := at("2026-10-26T23:59:59.999+08:00")
	task := save(t, s, actual.Add(-time.Hour))
	sign := &guardedSubmitSign{before: func() { time.Sleep(10 * time.Millisecond) }}
	checker := recordCheck(func(context.Context, string, time.Time) (bool, error) { return false, nil })
	r := NewRunner(s, sign, actual, checker)
	r.execute(context.Background(), task, actual, actual)
	runs, err := s.Runs("u")
	if err != nil || sign.calls != 0 || len(runs) != 1 || runs[0].Status != "cancelled" || !strings.Contains(runs[0].Message, "跨天") {
		t.Fatalf("submissions=%d runs=%#v error=%v", sign.calls, runs, err)
	}
}
