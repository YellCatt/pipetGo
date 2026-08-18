package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"

	"pipetGo/internal/logger"
	"pipetGo/internal/reporting"
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

	// 模板配置
	DayShowSummary   bool
	DayShowGrowth    bool
	DayShowError     bool
	DayShowSlow      bool
	DayShowAlert     bool
	WeekShowSummary  bool
	WeekShowGrowth   bool
	WeekShowError    bool
	WeekShowSlow     bool
	WeekShowAlert    bool
	MonthShowSummary bool
	MonthShowGrowth  bool
	MonthShowError   bool
	MonthShowSlow    bool
	MonthShowAlert   bool
	YearShowSummary  bool
	YearShowGrowth   bool
	YearShowError    bool
	YearShowSlow     bool
	YearShowAlert    bool
}

type TestRunner func(ctx context.Context)

func Start(ctx context.Context, dataDir string, cfg Config, runner TestRunner) {
	hasReport := cfg.WeeklyEnabled || cfg.MonthlyEnabled || cfg.YearlyEnabled || cfg.DailyEnabled

	if runner == nil {
		logger.Info("调度器未启动: 未提供测试运行器")
		return
	}

	loadState(dataDir)
	running = true

	sendHour, sendMinute := parseSendTime(cfg.SendTime)

	logger.Info("调度器配置汇总",
		zap.Int("报告发送时间", sendHour*60+sendMinute),
		zap.Int("测试间隔分钟", cfg.TestIntervalMinutes),
		zap.Bool("日报启用", cfg.DailyEnabled),
		zap.Bool("周报启用", cfg.WeeklyEnabled),
		zap.Bool("月报启用", cfg.MonthlyEnabled),
		zap.Bool("年报启用", cfg.YearlyEnabled))

	if hasReport {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error(fmt.Sprintf("报告调度器异常已恢复: %v", r))
				}
			}()
			logger.Info(fmt.Sprintf("报告调度器已启动，每天 %02d:%02d 发送报告", sendHour, sendMinute))
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					now := timeutil.Now()
					logger.Debug("报告调度器心跳",
						zap.String("当前时间", now.Format("2006-01-02 15:04:05")),
						zap.Int("目标发送时间", sendHour*60+sendMinute),
						zap.Int("当前分钟", now.Hour()*60+now.Minute()))
					checkAndSendReports(dataDir, cfg, sendHour, sendMinute)
				case <-ctx.Done():
					logger.Info("报告调度器已停止")
					return
				}
			}
		}()
	} else {
		logger.Info("所有报告类型均已禁用，报告调度器未启动")
	}

	intervalMinutes := cfg.TestIntervalMinutes
	if intervalMinutes <= 0 {
		intervalMinutes = 1440
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error(fmt.Sprintf("测试调度器异常已恢复: %v", r))
			}
		}()
		interval := time.Duration(intervalMinutes) * time.Minute
		logger.Info(fmt.Sprintf("测试调度器已启动，每 %d 分钟执行一次测试", intervalMinutes))

		select {
		case <-time.After(interval):
		case <-ctx.Done():
			logger.Info("测试调度器已停止")
			return
		}

		for {
			cycleStart := timeutil.Now()
			logger.Info("执行定时测试周期...")
			runner(ctx)
			elapsed := time.Since(cycleStart)

			if elapsed < interval {
				waitTime := interval - elapsed
				logger.Info(fmt.Sprintf("测试周期完成，耗时 %v，等待 %v 后开始下一周期", elapsed.Round(time.Second), waitTime.Round(time.Second)))
				select {
				case <-time.After(waitTime):
				case <-ctx.Done():
					logger.Info("测试调度器已停止")
					return
				}
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
		logger.Debug("未找到调度状态文件，使用默认状态",
			zap.String("路径", path),
			zap.Error(err))
		return
	}
	if err := json.Unmarshal(data, &state); err != nil {
		logger.Error("解析调度状态文件失败",
			zap.String("路径", path),
			zap.Error(err))
		return
	}
	logger.Info("调度状态已加载",
		zap.String("路径", path),
		zap.String("上次日报", state.LastSentDaily),
		zap.String("上次周报", state.LastSentWeekly),
		zap.String("上次月报", state.LastSentMonthly),
		zap.String("上次年报", state.LastSentYearly))
}

func saveState(dataDir string) {
	path := stateFilePath(dataDir)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		logger.Error("序列化调度状态失败", zap.Error(err))
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		logger.Error("写入调度状态文件失败", zap.String("路径", path), zap.Error(err))
		return
	}
	logger.Debug("调度状态已保存",
		zap.String("路径", path),
		zap.String("当前日报", state.LastSentDaily))
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

	logger.Debug("到达报告发送时间窗口",
		zap.Int("当前小时", now.Hour()),
		zap.Int("当前分钟", now.Minute()),
		zap.Int("目标小时", sendHour),
		zap.Int("目标分钟", sendMinute),
		zap.Bool("日报启用", cfg.DailyEnabled),
		zap.Bool("周报启用", cfg.WeeklyEnabled),
		zap.Bool("月报启用", cfg.MonthlyEnabled),
		zap.Bool("年报启用", cfg.YearlyEnabled))

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
		logger.Info("发送日报",
			zap.String("日期", dayKey),
			zap.String("上次已发送", state.LastSentDaily))
		dayTmpl := reporting.NewReportTemplate("日报", "本日", cfg.DayShowSummary, cfg.DayShowGrowth, cfg.DayShowError, cfg.DayShowSlow, cfg.DayShowAlert)
		if err := reporting.SendDailyReportEmailWithTemplate(consecutiveFailN, topSlowN, dayTmpl); err != nil {
			logger.Error("发送日报失败",
				zap.String("日期", dayKey),
				zap.Error(err))
		} else {
			state.LastSentDaily = dayKey
			sent = true
			logger.Info("日报发送成功", zap.String("日期", dayKey))
		}
	} else {
		logger.Debug("跳过日报",
			zap.Bool("启用", cfg.DailyEnabled),
			zap.String("日期", dayKey),
			zap.String("上次已发送", state.LastSentDaily))
	}

	if weekday == 1 && cfg.WeeklyEnabled && state.LastSentWeekly != weekKey {
		logger.Info("发送周报...")
		weekTmpl := reporting.NewReportTemplate("周报", "本周", cfg.WeekShowSummary, cfg.WeekShowGrowth, cfg.WeekShowError, cfg.WeekShowSlow, cfg.WeekShowAlert)
		if err := reporting.SendWeeklyReportEmailWithTemplate(consecutiveFailN, topSlowN, weekTmpl); err != nil {
			logger.Error("发送周报失败: " + err.Error())
		} else {
			state.LastSentWeekly = weekKey
			sent = true
		}
	}

	dayOfMonth := now.Day()
	if dayOfMonth == 1 && cfg.MonthlyEnabled && state.LastSentMonthly != monthKey {
		logger.Info("发送月报...")
		monthTmpl := reporting.NewReportTemplate("月报", "本月", cfg.MonthShowSummary, cfg.MonthShowGrowth, cfg.MonthShowError, cfg.MonthShowSlow, cfg.MonthShowAlert)
		if err := reporting.SendMonthlyReportEmailWithTemplate(consecutiveFailN, topSlowN, monthTmpl); err != nil {
			logger.Error("发送月报失败: " + err.Error())
		} else {
			state.LastSentMonthly = monthKey
			sent = true
		}
	}

	if dayOfMonth == 1 && now.Month() == time.January && cfg.YearlyEnabled && state.LastSentYearly != yearKey {
		logger.Info("发送年报...")
		yearTmpl := reporting.NewReportTemplate("年报", "本年", cfg.YearShowSummary, cfg.YearShowGrowth, cfg.YearShowError, cfg.YearShowSlow, cfg.YearShowAlert)
		if err := reporting.SendYearlyReportEmailWithTemplate(consecutiveFailN, topSlowN, yearTmpl); err != nil {
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
