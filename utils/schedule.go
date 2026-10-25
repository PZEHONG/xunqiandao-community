package utils

import (
	"context"
	"fmt"
	"log"

	"github.com/robfig/cron/v3"
)

type ScheduleTask struct {
	ID        int
	Account   string
	Address   string
	Latitude  string
	Longitude string
	Province  string
	City      string
	Remark    string
	Comment   string
	CronExpr  string
	Enabled   int
}

// SignFunc 定义一个函数类型，用于执行签到操作
type SignFunc func(ctx context.Context, account, address, latitude, longitude, province, city, remark, comment string) error

// LoadSchedules 从数据库加载所有定时任务
func LoadSchedules() ([]ScheduleTask, error) {
	rows, err := db.Query(`SELECT id, account, address, latitude, longitude, province, city, remark, comment, cron_expr, enabled FROM schedules WHERE enabled=1`)
	if err != nil {
		return nil, fmt.Errorf("查询定时任务失败: %v", err)
	}
	defer rows.Close()

	var tasks []ScheduleTask
	for rows.Next() {
		var t ScheduleTask
		err := rows.Scan(&t.ID, &t.Account, &t.Address, &t.Latitude, &t.Longitude, &t.Province, &t.City, &t.Remark, &t.Comment, &t.CronExpr, &t.Enabled)
		if err != nil {
			return nil, fmt.Errorf("读取行数据失败: %v", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// InitScheduler 初始化并启动定时任务调度
func InitScheduler(signFunc SignFunc) (*cron.Cron, error) {
	c := cron.New()
	tasks, err := LoadSchedules()
	if err != nil {
		return nil, err
	}

	for _, task := range tasks {
		// 将任务添加到cron调度器中
		t := task
		_, err := c.AddFunc(t.CronExpr, func() {
			// 执行签到操作
			ctx := context.Background()
			log.Printf("开始执行定时签到任务[%d]，账号：%s\n", t.ID, t.Account)
			err := signFunc(ctx, t.Account, t.Address, t.Latitude, t.Longitude, t.Province, t.City, t.Remark, t.Comment)
			if err != nil {
				log.Printf("定时签到任务[%d]执行失败: %v\n", t.ID, err)
			} else {
				log.Printf("定时签到任务[%d]执行成功\n", t.ID)
			}
		})
		if err != nil {
			log.Printf("添加定时任务[%d]失败: %v\n", t.ID, err)
		}
	}

	c.Start()
	return c, nil
}

// PerformSign 执行实际的签到操作
func PerformSign(ctx context.Context, account string, address string, latitude string, longitude string, province string, city string, remark string, comment string) error {
	// 检查必要参数是否存在
	if account == "" || address == "" {
		return fmt.Errorf("账号和地址不能为空")
	}

	// 模拟签到请求或执行签到逻辑
	// 例如，这里可以通过 HTTP 请求发送签到数据到服务器
	log.Print("执行已配置的签到任务")

	// 模拟签到成功逻辑
	// 如果需要，可以将签到结果写入数据库
	err := LogSignResult(account, address, "签到成功")
	if err != nil {
		return fmt.Errorf("记录签到结果失败: %v", err)
	}

	return nil
}

// LogSignResult 记录签到结果到数据库
func LogSignResult(account, address, result string) error {
	if db == nil {
		return fmt.Errorf("数据库未初始化")
	}

	insertSQL := `
    INSERT INTO sign_logs (account, address, result, timestamp)
    VALUES (?, ?, ?, datetime('now'))
    `
	_, err := db.Exec(insertSQL, account, address, result)
	if err != nil {
		return fmt.Errorf("写入签到日志失败: %v", err)
	}
	return nil
}
