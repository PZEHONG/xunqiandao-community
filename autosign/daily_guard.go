package autosign

import (
	"context"
	"errors"
	"log"
	"time"
)

var errAlreadySignedToday = errors.New("今日已有签到记录，本次未重复签到")
var errDailyRecordsUnverified = errors.New("无法核验今日签到记录，本次未提交；请查看习讯云记录后处理")
var errDailySignDayChanged = errors.New("准备期间已跨天，本次未提交；不会补签")

// Successful runs survive task deletion and cover delayed official visibility.
// Official details additionally cover manual signs made outside this service.
func (r *Runner) checkDailyRecords(ctx context.Context, t Task, day time.Time) error {
	if err := ctx.Err(); err != nil {
		return errDailyRecordsUnverified
	}
	var exists bool
	err := r.store.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM auto_sign_runs WHERE account=? AND date=? AND status IN ('success','already_signed'))`, t.Account, day.In(Shanghai).Format("2006-01-02")).Scan(&exists)
	if err != nil {
		log.Printf("[autosign] task_id=%d stage=daily_check status=failed error_class=%T", t.ID, err)
		return errDailyRecordsUnverified
	}
	if exists {
		return errAlreadySignedToday
	}
	if r.checker == nil {
		return errDailyRecordsUnverified
	}
	exists, err = r.checker.HasSignInOn(ctx, t.Account, day)
	if err != nil {
		log.Printf("[autosign] task_id=%d stage=daily_check status=failed error_class=%T", t.ID, err)
		return errDailyRecordsUnverified
	}
	if exists {
		return errAlreadySignedToday
	}
	return nil
}
