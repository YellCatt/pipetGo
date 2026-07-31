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
	hasTest := cfg.TestIntervalMinutes > 0 && runner != nil

	if !hasReport && !hasTest {
		logger.Info("Scheduler not started: no reports or test scheduling enabled")
		return
	}

	loadState(dataDir)

	if hasReport {
		go func() {
			logger.Info(fmt.Sprintf("Report scheduler started, sending reports at %02d:%02d", sendHour, sendMinute))
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()

			running = true
			for range ticker.C {
				checkAndSendReports(dataDir, cfg)
			}
		}()
	}

	if hasTest {
		running = true
		go func() {
			interval := time.Duration(cfg.TestIntervalMinutes) * time.Minute
			logger.Info(fmt.Sprintf("Test scheduler started, running tests every %d minutes", cfg.TestIntervalMinutes))
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for range ticker.C {
				logger.Info("Running scheduled test cycle...")
				runner()
			}
		}()
	}
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
		logger.Info("Sending weekly report...")
		if err := email.SendWeeklyReportEmail(consecutiveFailN, topSlowN); err != nil {
			logger.Error("Failed to send weekly report: " + err.Error())
		} else {
			state.LastSentWeekly = weekKey
			sent = true
		}
	}

	dayOfMonth := now.Day()
	if dayOfMonth == 1 && cfg.MonthlyEnabled && state.LastSentMonthly != monthKey {
		logger.Info("Sending monthly report...")
		if err := email.SendMonthlyReportEmail(consecutiveFailN, topSlowN); err != nil {
			logger.Error("Failed to send monthly report: " + err.Error())
		} else {
			state.LastSentMonthly = monthKey
			sent = true
		}
	}

	if dayOfMonth == 1 && now.Month() == time.January && cfg.YearlyEnabled && state.LastSentYearly != yearKey {
		logger.Info("Sending yearly report...")
		if err := email.SendYearlyReportEmail(consecutiveFailN, topSlowN); err != nil {
			logger.Error("Failed to send yearly report: " + err.Error())
		} else {
			state.LastSentYearly = yearKey
			sent = true
		}
	}

	if sent {
		saveState(dataDir)
	}
}
