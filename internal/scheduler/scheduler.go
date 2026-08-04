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
	stateFile = "report_schedule_state.json"
)

type scheduleState struct {
	LastSentWeekly  string `json:"last_sent_weekly"`
	LastSentMonthly string `json:"last_sent_monthly"`
	LastSentYearly  string `json:"last_sent_yearly"`
	LastSentDaily   string `json:"last_sent_daily"`
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
	DailyEnabled        bool
	TestIntervalMinutes int
	SendTime            string
}

type TestRunner func()

func Start(dataDir string, cfg Config, runner TestRunner) {
	hasReport := cfg.WeeklyEnabled || cfg.MonthlyEnabled || cfg.YearlyEnabled || cfg.DailyEnabled

	if runner == nil {
		logger.Info("调度器未启动: 未提供测试运行器")
		return
	}

	loadState(dataDir)
	running = true

	sendHour, sendMinute := parseSendTime(cfg.SendTime)

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
				checkAndSendReports(dataDir, cfg, sendHour, sendMinute)
			}
		}()
	}

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

func parseSendTime(sendTime string) (int, int) {
	if sendTime == "" {
		return 5, 0
	}
	var h, m int
	n, err := fmt.Sscanf(sendTime, "%d:%d", &h, &m)
	if err != nil || n != 2 {
		logger.Warn(fmt.Sprintf("报告发送时间格式无效: %s，使用默认值 05:00", sendTime))
		return 5, 0
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		logger.Warn(fmt.Sprintf("报告发送时间超出范围: %s，使用默认值 05:00", sendTime))
		return 5, 0
	}
	return h, m
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

func checkAndSendReports(dataDir string, cfg Config, sendHour, sendMinute int) {
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
	dayKey := now.Format("2006-01-02")

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

	if cfg.DailyEnabled && state.LastSentDaily != dayKey {
		logger.Info("发送日报...")
		if err := email.SendDailyReportEmail(consecutiveFailN, topSlowN); err != nil {
			logger.Error("发送日报失败: " + err.Error())
		} else {
			state.LastSentDaily = dayKey
			sent = true
		}
	}

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
