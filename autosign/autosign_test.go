package autosign

import (
	"context"
	"database/sql"
	"errors"
	_ "github.com/mattn/go-sqlite3"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"xixunyunsign/service"
)

func TestTaskAndHistoryPersistAcrossDatabaseReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.db")
	db, e := sql.Open("sqlite3", path)
	if e != nil {
		t.Fatal(e)
	}
	db.Exec(`CREATE TABLE users(account TEXT PRIMARY KEY,school_id REAL,token TEXT);INSERT INTO users VALUES('u',1,'test-token')`)
	s, e := NewStore(db)
	if e != nil {
		t.Fatal(e)
	}
	start := at("2026-10-15T00:59:58Z")
	seedPublicBindings(t, s)
	task := save(t, s, start)
	f := &fakeSign{}
	testRunner(s, f, start).Tick(context.Background(), start.Add(3*time.Second))
	db.Close()
	db, e = sql.Open("sqlite3", path)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s, e = NewStore(db)
	if e != nil {
		t.Fatal(e)
	}
	tasks, _ := s.Tasks("u")
	runs, _ := s.Runs("u")
	if len(tasks) != 1 || !tasks[0].Enabled || tasks[0].Address != task.Address || len(runs) != 1 {
		t.Fatalf("persistence failed: %#v %#v", tasks, runs)
	}
	testRunner(s, f, start).Tick(context.Background(), start.Add(4*time.Second))
	if f.calls != 1 {
		t.Fatal("reopening repeated submitted task")
	}
}

func TestSameAccountLaterSlotWaitsThenSkipsAfterSuccess(t *testing.T) {
	s := testStore(t)
	start := at("2026-10-15T00:59:58Z")
	save(t, s, start)
	next := sample()
	next.SchoolID, _ = s.School("u")
	next.Time = "09:01"
	if _, e := s.Save(next, start); e != nil {
		t.Fatal(e)
	}
	f := &slowSign{entered: make(chan string, 2), release: make(chan struct{})}
	r := testRunner(s, f, start)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Poll(ctx, start.Add(3*time.Second))
	select {
	case <-f.entered:
	case <-time.After(time.Second):
		t.Fatal("first task not started")
	}
	r.Poll(ctx, start.Add(30*time.Second))
	r.Poll(ctx, start.Add(61*time.Second))
	r.Poll(ctx, start.Add(63*time.Second))
	close(f.release)
	r.workers.Wait()
	runs, e := s.Runs("u")
	if e != nil || len(runs) != 2 {
		t.Fatalf("later same-account slot lost: %#v %v", runs, e)
	}
	if runs[0].Status != "already_signed" || runs[1].Status != "success" || len(f.entered) != 0 {
		t.Fatalf("later task must skip after earlier success: %#v", runs)
	}
}

type slowSign struct {
	service.SignService
	entered chan string
	release chan struct{}
}

func (f *slowSign) Remarks(context.Context, string) ([]service.SignRemark, error) {
	return []service.SignRemark{{Key: "0", Value: "上班"}}, nil
}
func (f *slowSign) SignInWithOptions(ctx context.Context, account, address, name, lat, lon string, o service.SignOptions) (string, error) {
	f.entered <- account
	select {
	case <-f.release:
		return "ok", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
func TestSlowExecutionDoesNotBlockLaterPolling(t *testing.T) {
	s := testStore(t)
	start := at("2026-10-15T00:59:58Z")
	save(t, s, start)
	next := sample()
	next.Account = "v"
	next.SchoolID, _ = s.School("v")
	next.Time = "09:01"
	if _, e := s.Save(next, start); e != nil {
		t.Fatal(e)
	}
	f := &slowSign{entered: make(chan string, 2), release: make(chan struct{})}
	r := testRunner(s, f, start)
	defer func() { close(f.release); r.workers.Wait() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Poll(ctx, start.Add(3*time.Second))
	select {
	case <-f.entered:
	case <-time.After(time.Second):
		t.Fatal("first task not dispatched")
	}
	r.Poll(ctx, start.Add(30*time.Second))
	r.Poll(ctx, start.Add(61*time.Second))
	r.Poll(ctx, start.Add(63*time.Second))
	select {
	case who := <-f.entered:
		if who != "v" {
			t.Fatal(who)
		}
	case <-time.After(time.Second):
		t.Fatal("slow request blocked later time slot")
	}
}
func TestClaimSeparatesScheduledDayAndActualStart(t *testing.T) {
	s := testStore(t)
	start := at("2026-10-15T00:59:58Z")
	task := save(t, s, start)
	actual := start.Add(3 * time.Minute)
	ok, e := s.claimAt(task, start.Add(3*time.Second), actual)
	if e != nil || !ok {
		t.Fatal(e)
	}
	if e = s.recover(actual.Add(time.Second)); e != nil {
		t.Fatal(e)
	}
	runs, _ := s.Runs("u")
	if runs[0].Status != "running" {
		t.Fatal("active execution prematurely recovered")
	}
}

type fakeSign struct {
	service.SignService
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeSign) SignInWithOptions(_ context.Context, account, address, name, lat, lon string, o service.SignOptions) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return "平台确认成功", f.err
}
func (f *fakeSign) Remarks(context.Context, string) ([]service.SignRemark, error) {
	return []service.SignRemark{{Key: "0", Value: "上班"}}, nil
}
func testStore(t *testing.T) *Store {
	t.Helper()
	d, e := sql.Open("sqlite3", ":memory:")
	if e != nil {
		t.Fatal(e)
	}
	d.SetMaxOpenConns(1)
	t.Cleanup(func() { d.Close() })
	_, e = d.Exec(`CREATE TABLE users(account TEXT PRIMARY KEY,school_id REAL,token TEXT); INSERT INTO users VALUES('u',1,'test-token'),('v',2,'test-token')`)
	if e != nil {
		t.Fatal(e)
	}
	s, e := NewStore(d)
	if e != nil {
		t.Fatal(e)
	}
	seedPublicBindings(t, s)
	return s
}
func at(s string) time.Time {
	v, e := time.Parse(time.RFC3339, s)
	if e != nil {
		panic(e)
	}
	return v
}
func sample() Task {
	return Task{Account: "u", SchoolID: "1.0", Name: "上班", Address: "广东省广州市参考地址", AddressName: "参考地点", Latitude: "23.1", Longitude: "113.2", Remark: "0", RemarkLabel: "上班", Time: "09:00", Enabled: true}
}
func save(t *testing.T, s *Store, now time.Time) Task {
	t.Helper()
	v := sample()
	v.SchoolID, _ = s.School("u")
	v, e := s.Save(v, now)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestDailyShanghaiAndNoRepeat(t *testing.T) {
	s := testStore(t)
	start := at("2026-10-15T00:59:58Z")
	task := save(t, s, start)
	f := &fakeSign{}
	r := testRunner(s, f, start)
	r.Tick(context.Background(), start.Add(3*time.Second))
	r.Tick(context.Background(), start.Add(5*time.Second))
	if f.calls != 1 {
		t.Fatalf("calls %d", f.calls)
	}
	runs, e := s.Runs("u")
	if e != nil || len(runs) != 1 || runs[0].Date != "2026-10-15" || runs[0].Status != "success" {
		t.Fatalf("runs %#v %v", runs, e)
	}
	task.Time = "09:01"
	task, e = s.Save(task, start.Add(10*time.Second))
	if e != nil {
		t.Fatal(e)
	}
	r.last = start.Add(59 * time.Second)
	r.Tick(context.Background(), start.Add(63*time.Second))
	if f.calls != 1 {
		t.Fatal("editing repeated today's sign")
	}
	if next := task.NextRun(start.Add(10 * time.Second)); next.Hour() != 9 || next.Minute() != 1 {
		t.Fatal(next)
	}
}
func TestRestartAndSleepDoNotCatchUp(t *testing.T) {
	for _, mode := range []string{"restart", "sleep"} {
		t.Run(mode, func(t *testing.T) {
			s := testStore(t)
			start := at("2026-10-15T00:59:00Z")
			save(t, s, start)
			f := &fakeSign{}
			r := testRunner(s, f, start)
			if mode == "restart" {
				r = testRunner(s, f, start.Add(2*time.Minute))
			}
			r.Tick(context.Background(), start.Add(2*time.Minute+time.Second))
			if f.calls != 0 {
				t.Fatal("caught up")
			}
		})
	}
}
func TestPauseVersionIsolationAndDeleteHistory(t *testing.T) {
	s := testStore(t)
	start := at("2026-10-15T00:59:58Z")
	task := save(t, s, start)
	if _, e := s.SetEnabled("v", task.ID, task.Revision, false, start); e == nil {
		t.Fatal("other account changed task")
	}
	paused, e := s.SetEnabled("u", task.ID, task.Revision, false, start)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Save(task, start); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale update: %v", e)
	}
	f := &fakeSign{}
	r := testRunner(s, f, start)
	r.Tick(context.Background(), start.Add(3*time.Second))
	if f.calls != 0 {
		t.Fatal("paused ran")
	}
	resumed, e := s.SetEnabled("u", task.ID, paused.Revision, true, start.Add(5*time.Second))
	if e != nil {
		t.Fatal(e)
	}
	r.Tick(context.Background(), start.Add(6*time.Second))
	if f.calls != 0 {
		t.Fatal("resume caught up")
	}
	if e = s.Delete("u", task.ID, resumed.Revision); e != nil {
		t.Fatal(e)
	}
	tasks, _ := s.Tasks("u")
	if len(tasks) != 0 {
		t.Fatal(tasks)
	}
}
func TestFailureNeverRetriesAndInputFailure(t *testing.T) {
	for _, e := range []error{errors.New("network token-secret"), service.ErrSignInput} {
		s := testStore(t)
		start := at("2026-10-15T00:59:58Z")
		save(t, s, start)
		f := &fakeSign{err: e}
		r := testRunner(s, f, start)
		r.Tick(context.Background(), start.Add(3*time.Second))
		testRunner(s, f, start).Tick(context.Background(), start.Add(4*time.Second))
		runs, _ := s.Runs("u")
		want := "failed"
		if errors.Is(e, service.ErrSignInput) {
			want = "failed"
		}
		if f.calls != 1 || len(runs) != 1 || runs[0].Status != want {
			t.Fatalf("%d %#v", f.calls, runs)
		}
		if runs[0].Message == e.Error() {
			t.Fatal("untrusted error leaked")
		}
	}
}
func TestConcurrentClaimAndSchoolChange(t *testing.T) {
	s := testStore(t)
	start := at("2026-10-15T00:59:58Z")
	save(t, s, start)
	f := &fakeSign{}
	r1, r2 := testRunner(s, f, start), testRunner(s, f, start)
	var w sync.WaitGroup
	for _, r := range []*Runner{r1, r2} {
		w.Add(1)
		go func(r *Runner) { defer w.Done(); r.Tick(context.Background(), start.Add(3*time.Second)) }(r)
	}
	w.Wait()
	if f.calls != 1 {
		t.Fatal(f.calls)
	}
	s2 := testStore(t)
	save(t, s2, start)
	s2.db.Exec("UPDATE users SET school_id=9 WHERE account='u'")
	f2 := &fakeSign{}
	testRunner(s2, f2, start).Tick(context.Background(), start.Add(3*time.Second))
	if f2.calls != 0 {
		t.Fatal("changed school ran")
	}
}
func TestInvalidConfigAndRecovery(t *testing.T) {
	s := testStore(t)
	start := at("2026-10-15T00:59:58Z")
	for _, clock := range []string{"9:00", "24:00", "09:60", "", "00:00:01"} {
		v := sample()
		v.Time = clock
		if _, e := s.Save(v, start); e == nil {
			t.Fatal(clock)
		}
	}
	task := save(t, s, start)
	ok, e := s.claim(task, start.Add(2*time.Second))
	if e != nil || !ok {
		t.Fatalf("claim %v %v", ok, e)
	}
	if e = s.recover(start.Add(3 * time.Minute)); e != nil {
		t.Fatal(e)
	}
	runs, _ := s.Runs("u")
	if len(runs) != 1 || runs[0].Status != "failed" {
		t.Fatal(runs)
	}
	if e = s.Delete("u", task.ID, task.Revision); e != nil {
		t.Fatal(e)
	}
	runs, _ = s.Runs("u")
	if len(runs) != 1 {
		t.Fatal("history removed")
	}
}

// Existing scheduling tests use verified empty official history.
func testRunner(store *Store, sign service.SignService, now time.Time) *Runner {
	return NewRunner(store, sign, now, recordCheck(func(context.Context, string, time.Time) (bool, error) { return false, nil }))
}

func seedPublicBindings(t *testing.T, s *Store) {
	t.Helper()
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS platform_bindings(account TEXT PRIMARY KEY,user_id INTEGER NOT NULL,school_id TEXT NOT NULL,school_name TEXT NOT NULL DEFAULT ''); INSERT OR IGNORE INTO platform_bindings(account,user_id,school_id) VALUES('u',1,'1'),('v',2,'2')`)
	if err != nil {
		t.Fatal(err)
	}
}
