package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"pipetGo/internal/email"
	"pipetGo/internal/logger"
	"pipetGo/internal/timeutil"
)

const (
	sendHour   = 6
	sendMinute = 0
	stateFile  = "report_schedule_state.json"
)

type scheduleState struct {
	LastSentWeekly  string `json:"last_sent_weekly"`
	LastSentMonthly string `json:"last_sent_monthly"`
	LastSentYearly  string `json:"last_sent_yearly"`
}

var (
	stateMu sync.Mutex
	state   scheduleState
	running bool
)

type Config struct {
	ConsecutiveFailN    int
	TopSlowN            int
	WeeklyEnabled       bool
	MonthlyEnabled      bool
	YearlyEnabled       bool
	TestIntervalMinutes int
}

type TestRunner func()

func Start(dataDir string, cfg Config, runner TestRunner) {
	hasReport := cfg.WeeklyEnabled || cfg.MonthlyEnabled || cfg.YearlyEnabled

	if runner == nil {
		logger.Info("调度器未启动: 未提供测试运行器")
		return
	}

	loadState(dataDir)
	running = true

	if hasReport {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error(fmt.Sprintf("报告调度器 panic 已恢复: %v", r))
				}
			}()
			logger.Info(fmt.Sprintf("报告调度器已启动，每天 %02d:%02d 发送报告", sendHour, sendMinute))
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()

			for range ticker.C {
				checkAndSendReports(dataDir, cfg)
			}
		}()
	}

	// 测试循环：默认每24小时（1440分钟）执行一轮
	// 首轮由 runTests 直接执行，调度器先等待一个间隔后再开始循环
	// 如果本轮耗时超过间隔，则完成后立即开始下一轮
	intervalMinutes := cfg.TestIntervalMinutes
	if intervalMinutes <= 0 {
		intervalMinutes = 1440
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error(fmt.Sprintf("测试调度器 panic 已恢复: %v", r))
			}
		}()
		interval := time.Duration(intervalMinutes) * time.Minute
		logger.Info(fmt.Sprintf("测试调度器已启动，每 %d 分钟执行一次测试", intervalMinutes))

		// 首次等待一个完整间隔（首轮已由 runTests 执行）
		time.Sleep(interval)

		for {
			cycleStart := timeutil.Now()
			logger.Info("执行定时测试周期...")
			runner()
			elapsed := time.Since(cycleStart)

			if elapsed < interval {
				waitTime := interval - elapsed
				logger.Info(fmt.Sprintf("测试周期完成，耗时 %v，等待 %v 后开始下一周期", elapsed.Round(time.Second), waitTime.Round(time.Second)))
				time.Sleep(waitTime)
			} else {
				logger.Info(fmt.Sprintf("测试周期完成，耗时 %v（超过间隔），立即开始下一周期", elapsed.Round(time.Second)))
			}
		}
	}()
}

func IsRunning() bool {
	return running
}

func loadState(dataDir string) {
	path := stateFilePath(dataDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &state)
}

func saveState(dataDir string) {
	path := stateFilePath(dataDir)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0644)
}

func stateFilePath(dataDir string) string {
	if dataDir == "" {
		dataDir = "./sql"
	}
	return filepath.Join(dataDir, stateFile)
}

func checkAndSendReports(dataDir string, cfg Config) {
	now := timeutil.Now()
	if now.Hour() != sendHour || now.Minute() != sendMinute {
		return
	}

	stateMu.Lock()
	defer stateMu.Unlock()

	year, week := now.ISOWeek()
	weekKey := fmt.Sprintf("%d-W%02d", year, week)
	monthKey := now.Format("2006-01")
	yearKey := now.Format("2006")

	consecutiveFailN := cfg.ConsecutiveFailN
	if consecutiveFailN <= 0 {
		consecutiveFailN = 3
	}
	topSlowN := cfg.TopSlowN
	if topSlowN <= 0 {
		topSlowN = 10
	}

	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}

	sent := false

	if weekday == 1 && cfg.WeeklyEnabled && state.LastSentWeekly != weekKey {
		logger.Info("发送周报...")
		if err := email.SendWeeklyReportEmail(consecutiveFailN, topSlowN); err != nil {
			logger.Error("发送周报失败: " + err.Error())
		} else {
			state.LastSentWeekly = weekKey
			sent = true
		}
	}

	dayOfMonth := now.Day()
	if dayOfMonth == 1 && cfg.MonthlyEnabled && state.LastSentMonthly != monthKey {
		logger.Info("发送月报...")
		if err := email.SendMonthlyReportEmail(consecutiveFailN, topSlowN); err != nil {
			logger.Error("发送月报失败: " + err.Error())
		} else {
			state.LastSentMonthly = monthKey
			sent = true
		}
	}

	if dayOfMonth == 1 && now.Month() == time.January && cfg.YearlyEnabled && state.LastSentYearly != yearKey {
		logger.Info("发送年报...")
		if err := email.SendYearlyReportEmail(consecutiveFailN, topSlowN); err != nil {
			logger.Error("发送年报失败: " + err.Error())
		} else {
			state.LastSentYearly = yearKey
			sent = true
		}
	}

	if sent {
		saveState(dataDir)
	}
}
