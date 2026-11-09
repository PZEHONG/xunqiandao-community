package autosign

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"
	"xixunyunsign/service"
)

type Runner struct {
	store     *Store
	sign      service.SignService
	checker   service.DailySignChecker
	mu        sync.Mutex
	last      time.Time
	accounts  sync.Map
	workers   sync.WaitGroup
	verifying atomic.Bool
}

type precheckJob struct {
	task    Task
	attempt int
	now     time.Time
}

func NewRunner(store *Store, sign service.SignService, now time.Time, checker service.DailySignChecker) *Runner {
	return &Runner{store: store, sign: sign, last: now, checker: checker}
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
	groups, jobs := r.collect(now)
	dispatched := time.Now()
	var workers sync.WaitGroup
	for _, group := range groups {
		workers.Add(1)
		go func(list []Task) { defer workers.Done(); r.runGroup(ctx, list, now, dispatched) }(group)
	}
	for _, job := range jobs {
		workers.Add(1)
		go func(j precheckJob) { defer workers.Done(); r.runPrecheck(ctx, j) }(job)
	}
	workers.Wait()
	r.Verify(ctx, now)
}

// Poll only plans work. Network requests cannot block the next clock tick.
func (r *Runner) Poll(ctx context.Context, now time.Time) {
	groups, jobs := r.collect(now)
	dispatched := time.Now()
	for _, group := range groups {
		r.workers.Add(1)
		go func(list []Task) { defer r.workers.Done(); r.runGroup(ctx, list, now, dispatched) }(group)
	}
	for _, job := range jobs {
		r.workers.Add(1)
		go func(j precheckJob) { defer r.workers.Done(); r.runPrecheck(ctx, j) }(job)
	}
	r.workers.Add(1)
	go func() {
		defer r.workers.Done()
		r.Verify(ctx, now)
	}()
}
func (r *Runner) collect(now time.Time) (map[string][]Task, []precheckJob) {
	r.mu.Lock()
	defer r.mu.Unlock()
	previous := r.last
	r.last = now
	if r.store == nil || r.sign == nil || !now.After(previous) {
		return nil, nil
	}
	if r.store.recover(now) != nil {
		return nil, nil
	}
	// A restart or sleep is not an invitation to submit missed attendance.
	if now.Sub(previous) > 45*time.Second {
		if r.store.recordMissed(now) != nil {
			log.Printf("[autosign] stage=scheduler status=missed_record_failed")
		}
		return nil, nil
	}
	tasks, e := r.store.Tasks("")
	if e != nil {
		return nil, nil
	}
	groups := map[string][]Task{}
	prechecks := []precheckJob{}
	for _, t := range tasks {
		if !t.Enabled {
			continue
		}
		day := now.In(Shanghai)
		due, e := time.ParseInLocation("2006-01-02 15:04", day.Format("2006-01-02")+" "+t.Time, Shanghai)
		if e != nil {
			continue
		}
		if due.After(now) || !due.After(previous) || !due.After(t.NotBefore) {
			// Precheck windows are before the planned time, so this task is not due yet.
			for _, attempt := range []int{1, 2, 3} {
				window := due.Add(-time.Duration(20-5*attempt) * time.Minute)
				if !window.After(previous) || window.After(now) || !window.After(t.NotBefore) {
					continue
				}
				existing, found, err := r.store.Precheck(t.ID, day.Format("2006-01-02"))
				if err != nil || found && (existing.Status == "ready" || existing.Status == "already_signed" || existing.Attempts >= attempt) {
					continue
				}
				prechecks = append(prechecks, precheckJob{task: t, attempt: attempt, now: now})
			}
			continue
		}
		groups[t.Account] = append(groups[t.Account], t)
	}
	if r.store.recordMissed(now) != nil {
		log.Printf("[autosign] stage=scheduler status=missed_record_failed")
	}
	return groups, prechecks
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

type runOutcome struct {
	status  string
	message string
	kind    string
}

func (r *Runner) execute(ctx context.Context, t Task, now, actual time.Time) {
	started := time.Now()
	claimed, e := r.store.claimAt(t, now, actual)
	if e != nil || !claimed {
		log.Printf("[autosign] task_id=%d stage=claim status=not_claimed duplicate_or_guard=%t error_class=%T", t.ID, e == nil, e)
		return
	}
	stage := "preflight"
	submitted := false
	outcome := runOutcome{status: "failed", message: "账号或学校信息不可用，请重新连接并确认任务", kind: "preflight"}
	log.Printf("[autosign] task_id=%d revision=%d scheduled_time=%s stage=start status=running", t.ID, t.Revision, t.Time)
	defer func() {
		if p := recover(); p != nil {
			outcome = runOutcome{status: "pending_verification", message: "执行中断，无法确认是否已提交；请先查看官方签到记录，系统不会自动重试", kind: "interrupted"}
			if stage != "sign" {
				outcome = runOutcome{status: "failed", message: "执行中断，本次未提交签到；请查看官方记录", kind: "interrupted"}
			}
		}
		failureKind, httpStatus, code := "", 0, ""
		var failure *service.SignFailure
		if errors.As(e, &failure) {
			failureKind, httpStatus, code = failure.Kind, failure.HTTPStatus, failure.Code
		}
		finishError := r.store.finish(t, now, outcome.status, outcome.message, stage, outcome.kind)
		log.Printf("[autosign] task_id=%d stage=%s status=%s duration_ms=%d error_class=%T failure_kind=%q http_status=%d code=%q message=%q persistence_error_class=%T", t.ID, stage, outcome.status, time.Since(started).Milliseconds(), e, failureKind, httpStatus, code, outcome.message, finishError)
	}()
	if actual.Sub(now) > 2*time.Minute || actual.In(Shanghai).Format("2006-01-02") != now.In(Shanghai).Format("2006-01-02") {
		outcome = runOutcome{status: "missed", message: "等待过久或已跨天，本次跳过；不会补签", kind: "window_expired"}
		return
	}
	current, e := r.store.Task(t.Account, t.ID)
	if e != nil || !current.Enabled || current.Revision != t.Revision {
		outcome = runOutcome{status: "cancelled", message: "任务已暂停或修改，本次未提交", kind: "task_changed"}
		return
	}
	if e = r.store.CheckSchool(t); e != nil {
		outcome = runOutcome{status: "failed", message: "学校信息已变化，请编辑并重新确认任务", kind: "school_changed"}
		return
	}
	ctx = context.WithValue(ctx, service.SignTaskIDKey{}, t.ID)
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	stage = "daily_check"
	if e = r.checkDailyRecords(ctx, t, actual); e != nil {
		if errors.Is(e, errAlreadySignedToday) {
			outcome = runOutcome{status: "already_signed", message: e.Error(), kind: "already_signed"}
		} else {
			outcome = runOutcome{status: "failed", message: e.Error(), kind: "records_unverified"}
		}
		return
	}
	stage = "remarks"
	remarks, e := r.sign.Remarks(ctx, t.Account)
	if e != nil {
		outcome = runOutcome{status: "failed", message: "无法核验账号签到类型，请重新连接账号后检查任务", kind: failureKind(e)}
		var failure *service.SignFailure
		if errors.As(e, &failure) {
			outcome.message = failure.Message
		}
		if errors.Is(e, context.DeadlineExceeded) {
			outcome.message = "核验账号签到类型超时，未发送签到请求"
			outcome.kind = "query_timeout"
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
		outcome = runOutcome{status: "failed", message: "签到类型已变化，请编辑并重新确认任务", kind: "remark_changed"}
		return
	}
	current, e = r.store.Task(t.Account, t.ID)
	if e != nil || !current.Enabled || current.Revision != t.Revision {
		outcome = runOutcome{status: "cancelled", message: "任务已暂停或修改，本次未提交", kind: "task_changed"}
		return
	}
	if e = r.store.CheckSchool(t); e != nil {
		outcome = runOutcome{status: "failed", message: "学校信息已变化，请编辑并重新确认任务", kind: "school_changed"}
		return
	}
	stage = "sign"
	_, e = r.sign.SignInWithOptions(ctx, t.Account, t.Address, t.AddressName, t.Latitude, t.Longitude, service.SignOptions{Remark: t.Remark, Comment: t.Comment, ExpectedSchoolID: t.SchoolID, ExpectedRemarkLabel: t.RemarkLabel, OnSubmitting: func() { submitted = true }, BeforeSubmit: func() error {
		dayChanged := func() bool {
			return actual.Add(time.Since(started)).In(Shanghai).Format("2006-01-02") != now.In(Shanghai).Format("2006-01-02")
		}
		if dayChanged() {
			return errDailySignDayChanged
		}
		if err := r.checkDailyRecords(ctx, t, actual.Add(time.Since(started))); err != nil {
			return err
		}
		if dayChanged() {
			return errDailySignDayChanged
		}
		latest, err := r.store.Task(t.Account, t.ID)
		if err != nil || !latest.Enabled || latest.Revision != t.Revision {
			return service.ErrSignCancelled
		}
		return nil
	}})
	if e == nil {
		outcome = runOutcome{status: "success", message: "习讯云已确认签到成功", kind: ""}
		return
	}
	if errors.Is(e, errAlreadySignedToday) {
		outcome = runOutcome{status: "already_signed", message: errAlreadySignedToday.Error(), kind: "already_signed"}
		return
	}
	if errors.Is(e, errDailyRecordsUnverified) {
		outcome = runOutcome{status: "failed", message: errDailyRecordsUnverified.Error(), kind: "records_unverified"}
		return
	}
	if errors.Is(e, errDailySignDayChanged) {
		outcome = runOutcome{status: "cancelled", message: errDailySignDayChanged.Error(), kind: "day_changed"}
		return
	}
	if errors.Is(e, service.ErrSignCancelled) {
		outcome = runOutcome{status: "cancelled", message: "任务已暂停或修改，本次未提交", kind: "task_changed"}
		return
	}
	if errors.Is(e, service.ErrSignInput) || errors.Is(e, service.ErrSignAccount) {
		outcome = runOutcome{status: "failed", message: "签到信息或账号无效，请重新连接并编辑任务", kind: "input"}
		return
	}
	kind := failureKind(e)
	if !submitted || !ambiguousFailure(kind) {
		outcome = runOutcome{status: "failed", message: failedMessage(e, kind), kind: kind}
		return
	}
	outcome = runOutcome{status: "pending_verification", message: pendingMessage(kind), kind: kind}
}

func failureKind(e error) string {
	var failure *service.SignFailure
	if errors.As(e, &failure) {
		switch failure.Code {
		case "99999":
			return "upstream_99999"
		case "64033":
			return "interval_64033"
		}
		switch failure.Kind {
		case "timeout":
			return "timeout"
		case "transport_error":
			return "transport_error"
		case "response_read_error":
			return "response_read_error"
		case "invalid_json":
			return "invalid_json"
		case "login_required", "renewal_failed":
			return "login_required"
		default:
			return "upstream_rejected"
		}
	}
	if errors.Is(e, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(e, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(e, service.ErrSignInput) {
		return "input"
	}
	if errors.Is(e, service.ErrSignAccount) {
		return "account"
	}
	if errors.Is(e, service.ErrUpstreamLoginRequired) {
		return "login_required"
	}
	return "unknown"
}

func ambiguousFailure(kind string) bool {
	switch kind {
	case "timeout", "transport_error", "response_read_error", "invalid_json", "cancelled", "unknown":
		return true
	}
	return false
}

func failedMessage(e error, kind string) string {
	switch kind {
	case "upstream_99999":
		return "习讯云返回签到失败（返回码：99999），具体原因未明；请先查看官方签到记录，系统不会自动重试"
	case "interval_64033":
		return "习讯云提示每次签到时间间隔至少3小时（返回码：64033）；请先核对官方记录和学校规则，系统不会自动重试"
	case "login_required":
		return "习讯云登录已失效，请重新连接账号"
	case "upstream_rejected":
		var failure *service.SignFailure
		if errors.As(e, &failure) && failure.Message != "" {
			return failure.Message + "；请先查看官方签到记录，系统不会自动重试"
		}
		return "习讯云未接受本次签到；请先查看官方签到记录，系统不会自动重试"
	case "query_timeout":
		return "核验账号签到类型超时，未发送签到请求；请稍后检查账号连接"
	}
	var failure *service.SignFailure
	if errors.As(e, &failure) && failure.Message != "" {
		return failure.Message
	}
	return "执行失败，本次未提交签到；请检查账号连接和任务设置"
}

func pendingMessage(kind string) string {
	switch kind {
	case "timeout":
		return "请求超时，无法确认是否已提交；请先查看官方签到记录，系统不会自动重试"
	case "transport_error":
		return "连接习讯云失败，无法确认是否已提交；请先查看官方签到记录，系统不会自动重试"
	case "response_read_error", "invalid_json":
		return "习讯云响应异常，无法确认是否已提交；请先查看官方签到记录，系统不会自动重试"
	}
	return "执行结果无法确认；请先查看官方签到记录，系统不会自动重试"
}

func (r *Runner) runPrecheck(ctx context.Context, job precheckJob) {
	t := job.task
	date := job.now.In(Shanghai).Format("2006-01-02")
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	status, message := "ready", "预检查通过：任务、账号、会话、签到类型与当日记录均可核验"
	next := job.now.Add(5 * time.Minute)
	if job.attempt >= 3 {
		next = time.Time{}
	}
	if current, err := r.store.Task(t.Account, t.ID); err != nil || !current.Enabled || current.Revision != t.Revision {
		status, message = "cancelled", "任务已暂停或修改，预检查已取消"
	} else if err := r.store.CheckSchool(t); err != nil {
		status, message = "failed", "学校信息已变化，请编辑并重新确认任务"
	} else if err := r.checkDailyRecords(ctx, t, job.now); err != nil {
		if errors.Is(err, errAlreadySignedToday) {
			status, message = "already_signed", err.Error()
		} else {
			status, message = "unverified", err.Error()
		}
	} else if remarks, err := r.sign.Remarks(ctx, t.Account); err != nil {
		status, message = "session", "无法核验账号签到类型，请检查账号连接"
		var failure *service.SignFailure
		if errors.As(err, &failure) && failure.Message != "" {
			message = failure.Message
		}
	} else {
		valid := false
		for _, remark := range remarks {
			if remark.Key == t.Remark && remark.Value == t.RemarkLabel {
				valid = true
				break
			}
		}
		if !valid {
			status, message = "remark_changed", "签到类型已变化，请编辑并重新确认任务"
		}
	}
	if err := r.store.SavePrecheck(t.ID, date, status, message, job.attempt, job.now, next); err != nil {
		log.Printf("[autosign] task_id=%d stage=precheck status=persistence_failed error_class=%T", t.ID, err)
		return
	}
	log.Printf("[autosign] task_id=%d stage=precheck attempt=%d status=%s", t.ID, job.attempt, status)
}

// Verify performs only read-only official record checks for ambiguous submits.
// It never resubmits a sign-in.
func (r *Runner) Verify(ctx context.Context, now time.Time) {
	if r.store == nil || r.checker == nil || !r.verifying.CompareAndSwap(false, true) {
		return
	}
	defer r.verifying.Store(false)
	runs, e := r.store.PendingVerifications(now)
	if e != nil {
		return
	}
	groups := map[string][]Run{}
	for _, run := range runs {
		groups[run.Account] = append(groups[run.Account], run)
	}
	for account, list := range groups {
		lock, _ := r.accounts.LoadOrStore(account, &sync.Mutex{})
		mutex := lock.(*sync.Mutex)
		mutex.Lock()
		for _, run := range list {
			if ctx.Err() != nil {
				mutex.Unlock()
				return
			}
			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			day, err := time.ParseInLocation("2006-01-02", run.Date, Shanghai)
			found := false
			if err == nil {
				found, err = r.checker.HasSignInOn(checkCtx, run.Account, day)
			}
			cancel()
			if err != nil {
				found = false
			}
			if err := r.store.resolveVerification(run.ID, found, now); err != nil {
				log.Printf("[autosign] run_id=%d stage=verify status=persistence_failed error_class=%T", run.ID, err)
			} else {
				log.Printf("[autosign] run_id=%d stage=verify status=%s found=%t", run.ID, map[bool]string{true: "already_signed", false: "unconfirmed"}[found], found)
			}
		}
		mutex.Unlock()
	}
}
