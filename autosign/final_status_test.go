package autosign

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"xixunyunsign/service"
)

type panicSign struct{ fakeSign }

func (f *panicSign) SignInWithOptions(ctx context.Context, a, b, c, d, e string, o service.SignOptions) (string, error) {
	panic("private-token")
}
func TestFinalFailureStatusesAndRecovery(t *testing.T) {
	for _, mode := range []string{"transport", "timeout", "panic", "late", "recovery", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			s := testStore(t)
			start := at("2026-10-15T00:59:58Z")
			task := save(t, s, start)
			f := &fakeSign{err: errors.New("private-token")}
			r := NewRunner(s, f, start)
			switch mode {
			case "legacy":
				s.db.Exec("INSERT INTO auto_sign_runs(task_id,account,name,revision,date,scheduled_time,started_at,status,message) VALUES(1,'u','test',1,'2026-10-14','09:00',?,'unknown','未获得明确成功结果，结果待核对')", stamp(start))
				if _, e := NewStore(s.db); e != nil {
					t.Fatal(e)
				}
			case "recovery":
				s.claim(task, start)
				s.recover(start.Add(3 * time.Minute))
			case "late":
				r.execute(context.Background(), task, start.Add(3*time.Second), start.Add(3*time.Minute))
			case "panic":
				NewRunner(s, &panicSign{}, start).Tick(context.Background(), start.Add(3*time.Second))
			default:
				if mode == "timeout" {
					f.err = context.DeadlineExceeded
				}
				r.Tick(context.Background(), start.Add(3*time.Second))
			}
			runs, e := s.Runs("u")
			if e != nil || len(runs) != 1 || runs[0].Status != "failed" {
				t.Fatalf("want failed: %#v %v", runs, e)
			}
			if strings.Contains(runs[0].Message, "待核对") || strings.Contains(runs[0].Message, "private-token") {
				t.Fatal(runs[0].Message)
			}
			if mode == "transport" || mode == "timeout" {
				NewRunner(s, f, start).Tick(context.Background(), start.Add(4*time.Second))
				if f.calls != 1 {
					t.Fatal("repeated submission")
				}
			}
		})
	}
}
