package autosign

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"
	"xixunyunsign/service"
)

type Runner struct {
	store    *Store
	sign     service.SignService
	mu       sync.Mutex
	last     time.Time
	accounts sync.Map
	workers  sync.WaitGroup
}

func NewRunner(store *Store, sign service.SignService, now time.Time) *Runner {
	return &Runner{store: store, sign: sign, last: now}
}
func (r *Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.Poll(ctx, time.Now())
		}
	}
}
func (r *Runner) Tick(ctx context.Context, now time.Time) {
	groups := r.collect(now)
	dispatched := time.Now()
	var workers sync.WaitGroup
	for _, group := range groups {
		workers.Add(1)
		go func(list []Task) { defer workers.Done(); r.runGroup(ctx, list, now, dispatched) }(group)
	}
	workers.Wait()
}

// Poll only plans work. Network requests cannot block the next clock tick.
func (r *Runner) Poll(ctx context.Context, now time.Time) {
	groups := r.collect(now)
	dispatched := time.Now()
	for _, group := range groups {
		r.workers.Add(1)
		go func(list []Task) { defer r.workers.Done(); r.runGroup(ctx, list, now, dispatched) }(group)
	}
}
func (r *Runner) collect(now time.Time) map[string][]Task {
	r.mu.Lock()
	defer r.mu.Unlock()
	previous := r.last
	r.last = now
	if r.store == nil || r.sign == nil || !now.After(previous) || r.store.recover(now) != nil {
		return nil
	}
	// A restart or sleep is not an invitation to submit missed attendance.
	if now.Sub(previous) > 45*time.Second {
		return nil
	}
	tasks, e := r.store.Tasks("")
	if e != nil {
		return nil
	}
	groups := map[string][]Task{}
	for _, t := range tasks {
		if !t.Enabled {
			continue
		}
		day := now.In(Shanghai)
		due, e := time.ParseInLocation("2006-01-02 15:04", day.Format("2006-01-02")+" "+t.Time, Shanghai)
		if e != nil || due.After(now) || !due.After(previous) || !due.After(t.NotBefore) {
			continue
		}
		groups[t.Account] = append(groups[t.Account], t)
	}
	return groups
}
func (r *Runner) runGroup(ctx context.Context, list []Task, planned, dispatched time.Time) {
	if len(list) == 0 {
		return
	}
	lock, _ := r.accounts.LoadOrStore(list[0].Account, &sync.Mutex{})
	mutex := lock.(*sync.Mutex)
	mutex.Lock()
	defer mutex.Unlock()
	for _, t := range list {
		if ctx.Err() != nil {
			return
		}
		actual := planned.Add(time.Since(dispatched))
		r.execute(ctx, t, planned, actual)
	}
}
func (r *Runner) execute(ctx context.Context, t Task, now, actual time.Time) {
	started := time.Now()
	claimed, e := r.store.claimAt(t, now, actual)
	if e != nil || !claimed {
		log.Printf("[autosign] task_id=%d stage=claim status=not_claimed duplicate_or_guard=%t error_class=%T", t.ID, e == nil, e)
		return
	}
	stage := "preflight"
	log.Printf("[autosign] task_id=%d revision=%d scheduled_time=%s stage=start status=running", t.ID, t.Revision, t.Time)
	status, message := "failed", "账号或学校信息不可用，请重新连接并确认任务"
	defer func() {
		if recover() != nil {
			status = "failed"
			message = "执行中断，未确认成功；可能已提交，请先查看官方签到记录"
			stage = "panic"
		}
		failureKind, httpStatus, code := "", 0, ""
		var failure *service.SignFailure
		if errors.As(e, &failure) {
			failureKind, httpStatus, code = failure.Kind, failure.HTTPStatus, failure.Code
		}
		finishError := r.store.finish(t, now, status, message)
		log.Printf("[autosign] task_id=%d stage=%s status=%s duration_ms=%d error_class=%T failure_kind=%q http_status=%d code=%q message=%q persistence_error_class=%T", t.ID, stage, status, time.Since(started).Milliseconds(), e, failureKind, httpStatus, code, message, finishError)
	}()
	if actual.Sub(now) > 2*time.Minute || actual.In(Shanghai).Format("2006-01-02") != now.In(Shanghai).Format("2006-01-02") {
		status = "failed"
		message = "等待过久或已跨天，本次跳过；不会补签"
		return
	}
	current, e := r.store.Task(t.Account, t.ID)
	if e != nil || !current.Enabled || current.Revision != t.Revision {
		status = "failed"
		message = "任务已暂停或修改，本次跳过"
		return
	}
	if e = r.store.CheckSchool(t); e != nil {
		return
	}
	ctx = context.WithValue(ctx, service.SignTaskIDKey{}, t.ID)
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	stage = "remarks"
	remarks, e := r.sign.Remarks(ctx, t.Account)
	if e != nil {
		message = "无法核验账号签到类型，请重新连接账号后检查任务"
		var failure *service.SignFailure
		if errors.As(e, &failure) {
			message = failure.Message
		}
		if errors.Is(e, context.DeadlineExceeded) {
			message = "核验账号签到类型超时，未发送签到请求"
		}
		return
	}
	valid := false
	for _, remark := range remarks {
		if remark.Key == t.Remark && remark.Value == t.RemarkLabel {
			valid = true
			break
		}
	}
	if !valid {
		message = "签到类型已变化，请编辑并重新确认任务"
		return
	}
	current, e = r.store.Task(t.Account, t.ID)
	if e != nil || !current.Enabled || current.Revision != t.Revision {
		status = "failed"
		message = "任务已暂停或修改，本次跳过"
		return
	}
	if e = r.store.CheckSchool(t); e != nil {
		return
	}
	stage = "sign"
	_, e = r.sign.SignInWithOptions(ctx, t.Account, t.Address, t.AddressName, t.Latitude, t.Longitude, service.SignOptions{Remark: t.Remark, Comment: t.Comment, ExpectedSchoolID: t.SchoolID, ExpectedRemarkLabel: t.RemarkLabel, BeforeSubmit: func() error {
		latest, err := r.store.Task(t.Account, t.ID)
		if err != nil || !latest.Enabled || latest.Revision != t.Revision {
			return service.ErrSignCancelled
		}
		return nil
	}})
	if e == nil {
		status = "success"
		message = "习讯云已确认签到成功"
		return
	}
	if errors.Is(e, service.ErrSignCancelled) {
		status = "failed"
		message = "任务已暂停或修改，本次跳过"
		return
	}
	if errors.Is(e, service.ErrSignInput) || errors.Is(e, service.ErrSignAccount) {
		message = "签到信息或账号无效，请重新连接并编辑任务"
		return
	}
	status = "failed"
	message = "执行失败：未确认成功；可能已提交，请先查看官方签到记录，系统不会自动重试"
	var failure *service.SignFailure
	if errors.As(e, &failure) {
		message = failure.Message
	}
	if errors.Is(e, context.DeadlineExceeded) {
		message = "请求超时，未确认成功；可能已提交，请先查看官方签到记录，系统不会自动重试"
	}
	if errors.Is(e, context.Canceled) {
		message = "执行被中断，未确认成功；可能已提交，请先查看官方签到记录，系统不会自动重试"
	}
	if errors.Is(e, service.ErrUpstreamLoginRequired) && failure == nil {
		message = "习讯云登录已失效，请重新连接账号"
	}
}
