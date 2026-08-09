package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"pipetGo/config"
	"pipetGo/internal/email"
	"pipetGo/internal/httpclient"
	"pipetGo/internal/logger"
	"pipetGo/internal/psv"
	"pipetGo/internal/reporting"
	"pipetGo/internal/scheduler"
	"pipetGo/internal/storage"
	"pipetGo/internal/testcase"
	"pipetGo/internal/timeutil"
	"pipetGo/internal/vars"

	"go.uber.org/zap"
)

type Options struct {
	Tags        string
	Rounds      int
	IntervalMs  int
	SendWeekly  bool
	SendMonthly bool
	SendYearly  bool
	SendDaily   bool
}

// NewAppContext 创建带信号监听的应用上下文
// 监听 SIGINT/SIGTERM，用于优雅退出
func NewAppContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case sig := <-sigCh:
			logger.Info("收到退出信号，正在优雅关闭...", zap.String("signal", sig.String()))
			cancel()
		case <-ctx.Done():
		}
		signal.Stop(sigCh)
	}()

	return ctx, cancel
}

func Init(ctx context.Context) {
	vars.Set("base_url", config.AppConfig.Target.BaseURL)
	logger.Debug("内置变量 base_url 已设置", zap.String("base_url", config.AppConfig.Target.BaseURL))

	if len(config.AppConfig.Vars) > 0 {
		vars.InitFromConfig(config.AppConfig.Vars)
		logger.Info("用户自定义变量加载完成", zap.Int("count", len(config.AppConfig.Vars)), zap.Any("vars", maskVars(config.AppConfig.Vars)))
	} else {
		logger.Info("未配置用户自定义变量")
	}
	logger.Info("当前可用变量", zap.Any("vars", vars.GetAll()))

	email.InitEmail(email.EmailConfig{
		Enabled:    config.AppConfig.Email.Enabled,
		FromEmail:  config.AppConfig.Email.From,
		ToEmail:    config.AppConfig.Email.To,
		AuthCode:   config.AppConfig.Email.AuthCode,
		SMTPServer: config.AppConfig.Email.SMTPServer,
		SMTPPort:   config.AppConfig.Email.SMTPPort,
		DeviceName: config.AppConfig.Test.DeviceName,
	})
	logger.Debug("邮件配置初始化完成", zap.Bool("enabled", config.AppConfig.Email.Enabled))

	scheduler.Start(
		ctx,
		config.AppConfig.Test.DataDir,
		scheduler.Config{
			ConsecutiveFailN:    config.AppConfig.Reporting.ConsecutiveFailN,
			TopSlowN:            config.AppConfig.Reporting.TopSlowN,
			WeeklyEnabled:       config.AppConfig.Reporting.WeeklyEnabled,
			MonthlyEnabled:      config.AppConfig.Reporting.MonthlyEnabled,
			YearlyEnabled:       config.AppConfig.Reporting.YearlyEnabled,
			DailyEnabled:        config.AppConfig.Reporting.DailyEnabled,
			TestIntervalMinutes: config.AppConfig.Test.ScheduleIntervalMinutes,
			SendTime:            config.AppConfig.Reporting.SendTime,

			DayShowSummary:   config.AppConfig.Reporting.DayTemplate.ShowSummary,
			DayShowGrowth:    config.AppConfig.Reporting.DayTemplate.ShowGrowth,
			DayShowError:     config.AppConfig.Reporting.DayTemplate.ShowError,
			DayShowSlow:      config.AppConfig.Reporting.DayTemplate.ShowSlow,
			DayShowAlert:     config.AppConfig.Reporting.DayTemplate.ShowAlert,
			WeekShowSummary:  config.AppConfig.Reporting.WeekTemplate.ShowSummary,
			WeekShowGrowth:   config.AppConfig.Reporting.WeekTemplate.ShowGrowth,
			WeekShowError:    config.AppConfig.Reporting.WeekTemplate.ShowError,
			WeekShowSlow:     config.AppConfig.Reporting.WeekTemplate.ShowSlow,
			WeekShowAlert:    config.AppConfig.Reporting.WeekTemplate.ShowAlert,
			MonthShowSummary: config.AppConfig.Reporting.MonthTemplate.ShowSummary,
			MonthShowGrowth:  config.AppConfig.Reporting.MonthTemplate.ShowGrowth,
			MonthShowError:   config.AppConfig.Reporting.MonthTemplate.ShowError,
			MonthShowSlow:    config.AppConfig.Reporting.MonthTemplate.ShowSlow,
			MonthShowAlert:   config.AppConfig.Reporting.MonthTemplate.ShowAlert,
			YearShowSummary:  config.AppConfig.Reporting.YearTemplate.ShowSummary,
			YearShowGrowth:   config.AppConfig.Reporting.YearTemplate.ShowGrowth,
			YearShowError:    config.AppConfig.Reporting.YearTemplate.ShowError,
			YearShowSlow:     config.AppConfig.Reporting.YearTemplate.ShowSlow,
			YearShowAlert:    config.AppConfig.Reporting.YearTemplate.ShowAlert,
		},
		func(ctx context.Context) {
			_ = ExecuteTestCycle(ctx, nil, Options{})
		},
	)
	logger.Debug("调度器启动完成")

	config.RegisterOnChange(func(newCfg config.Config) {
		logger.UpdateLevel(newCfg.Log.Level)

		email.UpdateConfig(email.EmailConfig{
			Enabled:    newCfg.Email.Enabled,
			FromEmail:  newCfg.Email.From,
			ToEmail:    newCfg.Email.To,
			AuthCode:   newCfg.Email.AuthCode,
			SMTPServer: newCfg.Email.SMTPServer,
			SMTPPort:   newCfg.Email.SMTPPort,
			DeviceName: newCfg.Test.DeviceName,
		})

		vars.Set("base_url", newCfg.Target.BaseURL)

		if len(newCfg.Vars) > 0 {
			vars.InitFromConfig(newCfg.Vars)
		}

		logger.Info("配置已热更新",
			zap.String("log_level", newCfg.Log.Level),
			zap.Bool("email_enabled", newCfg.Email.Enabled),
			zap.String("base_url", newCfg.Target.BaseURL),
		)
	})

	logger.Debug("Init 执行完成")
}

func maskVars(vars map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range vars {
		result[k] = maskString(v)
	}
	return result
}

func maskString(s string) string {
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "***" + s[len(s)-4:]
}

func InitStorage() {
	httpclient.InitClient()
	if err := storage.InitDB(config.GetConfig().Test.DataDir); err != nil {
		logger.Warn("CSV 存储初始化失败", zap.Error(err))
	}
}

func RunReport(ctx context.Context) {
	cfg := config.GetConfig()
	reportingCfg := cfg.Reporting
	consecutiveFailN := reportingCfg.ConsecutiveFailN
	if consecutiveFailN <= 0 {
		consecutiveFailN = 3
	}
	topSlowN := reportingCfg.TopSlowN
	if topSlowN <= 0 {
		topSlowN = 10
	}

	deviceName := cfg.Test.DeviceName
	if deviceName == "" {
		hostname, _ := os.Hostname()
		deviceName = hostname
	}

	fmt.Print(reporting.GenerateASCIIReport(deviceName, consecutiveFailN, topSlowN))
	waitForScheduler(ctx)
}

func RunSendReports(ctx context.Context, opts Options) {
	cfg := config.GetConfig()
	reportingCfg := cfg.Reporting
	consecutiveFailN := reportingCfg.ConsecutiveFailN
	if consecutiveFailN <= 0 {
		consecutiveFailN = 3
	}
	topSlowN := reportingCfg.TopSlowN
	if topSlowN <= 0 {
		topSlowN = 10
	}

	if opts.SendWeekly {
		logger.Info("发送周报...")
		weekTmpl := reporting.NewReportTemplate("周报", "本周",
			reportingCfg.WeekTemplate.ShowSummary,
			reportingCfg.WeekTemplate.ShowGrowth,
			reportingCfg.WeekTemplate.ShowError,
			reportingCfg.WeekTemplate.ShowSlow,
			reportingCfg.WeekTemplate.ShowAlert)
		if err := reporting.SendWeeklyReportEmailWithTemplate(consecutiveFailN, topSlowN, weekTmpl); err != nil {
			logger.Error("发送周报失败", zap.Error(err))
		} else {
			logger.Info("周报发送成功")
		}
	}

	if opts.SendDaily {
		logger.Info("发送日报...")
		dayTmpl := reporting.NewReportTemplate("日报", "本日",
			reportingCfg.DayTemplate.ShowSummary,
			reportingCfg.DayTemplate.ShowGrowth,
			reportingCfg.DayTemplate.ShowError,
			reportingCfg.DayTemplate.ShowSlow,
			reportingCfg.DayTemplate.ShowAlert)
		if err := reporting.SendDailyReportEmailWithTemplate(consecutiveFailN, topSlowN, dayTmpl); err != nil {
			logger.Error("发送日报失败", zap.Error(err))
		} else {
			logger.Info("日报发送成功")
		}
	}

	if opts.SendMonthly {
		logger.Info("发送月报...")
		monthTmpl := reporting.NewReportTemplate("月报", "本月",
			reportingCfg.MonthTemplate.ShowSummary,
			reportingCfg.MonthTemplate.ShowGrowth,
			reportingCfg.MonthTemplate.ShowError,
			reportingCfg.MonthTemplate.ShowSlow,
			reportingCfg.MonthTemplate.ShowAlert)
		if err := reporting.SendMonthlyReportEmailWithTemplate(consecutiveFailN, topSlowN, monthTmpl); err != nil {
			logger.Error("发送月报失败", zap.Error(err))
		} else {
			logger.Info("月报发送成功")
		}
	}

	if opts.SendYearly {
		logger.Info("发送年报...")
		yearTmpl := reporting.NewReportTemplate("年报", "本年",
			reportingCfg.YearTemplate.ShowSummary,
			reportingCfg.YearTemplate.ShowGrowth,
			reportingCfg.YearTemplate.ShowError,
			reportingCfg.YearTemplate.ShowSlow,
			reportingCfg.YearTemplate.ShowAlert)
		if err := reporting.SendYearlyReportEmailWithTemplate(consecutiveFailN, topSlowN, yearTmpl); err != nil {
			logger.Error("发送年报失败", zap.Error(err))
		} else {
			logger.Info("年报发送成功")
		}
	}

	waitForScheduler(ctx)
}

func RunTests(ctx context.Context, paths []string, opts Options) {
	logger.Debug("RunTests 被调用", zap.Strings("paths", paths))
	_ = ExecuteTestCycle(ctx, paths, opts)
	logger.Debug("ExecuteTestCycle 执行完成")
	waitForScheduler(ctx)
	logger.Debug("waitForScheduler 返回")
}

func waitForScheduler(ctx context.Context) {
	if !scheduler.IsRunning() {
		return
	}
	logger.Info("守护进程模式: 调度器正在运行，保持进程存活...")
	<-ctx.Done()
	logger.Info("调度器被取消")
}

func ExecuteTestCycle(ctx context.Context, paths []string, opts Options) []testcase.TestResult {
	logger.Debug("ExecuteTestCycle 开始执行")
	cfg := config.GetConfig()
	logger.Debug("获取配置完成",
		zap.String("test_case_dir", fmt.Sprint(cfg.Test.TestCaseDir)),
		zap.String("data_dir", cfg.Test.DataDir))

	fmt.Print(reporting.StartupBanner(cfg.Test.DeviceName, cfg.Log.Output))

	httpclient.InitClient()
	logger.Debug("HTTP 客户端初始化完成")

	logger.Info("准备初始化 CSV 存储", zap.String("DataDir", cfg.Test.DataDir))
	if err := storage.InitDB(cfg.Test.DataDir); err != nil {
		logger.Warn("CSV 存储初始化失败", zap.Error(err))
	} else {
		logger.Info("CSV 存储初始化成功")

		count, err := storage.GetTotalExecutionCount()
		if err != nil {
			logger.Warn("获取执行计数失败", zap.Error(err))
		} else {
			logger.Info("找到历史执行记录", zap.Int("count", count))
		}
	}

	if len(paths) == 0 {
		paths = cfg.Test.TestCaseDir
	}
	logger.Debug("待解析的测试用例路径", zap.Strings("paths", paths))

	logger.Debug("开始解析 PSV/CSV 测试用例文件")
	testCases, err := psv.ParseFiles(paths)
	if err != nil {
		logger.Error("解析 PSV 文件失败", zap.Error(err))
		errorMsg := fmt.Sprintf("解析测试用例文件失败: %v", err)
		if sendErr := reporting.SendErrorReportEmail(errorMsg); sendErr != nil {
			logger.Warn("发送错误报告邮件失败", zap.Error(sendErr))
		}
		os.Exit(1)
	}
	logger.Debug("PSV 解析完成", zap.Int("count", len(testCases)))

	testcase.SetAllTestCases(testCases)

	var tags []string
	if opts.Tags != "" {
		tags = strings.Split(opts.Tags, ",")
		for i, tag := range tags {
			tags[i] = strings.TrimSpace(tag)
		}
	}

	totalTestCaseCount, totalChainCount, totalIndependentCount := testcase.CountTestSummary(testCases)

	testCases = testcase.FilterByTags(testCases, tags)
	logger.Debug("标签过滤后剩余", zap.Int("count", len(testCases)))

	if len(testCases) == 0 {
		logger.Debug("没有需要执行的测试用例，ExecuteTestCycle 返回")
		logger.Info("没有需要执行的测试用例")
		return nil
	}

	logger.Info("开始执行 API 测试", zap.Int("count", len(testCases)))

	estimatedDuration := calculateEstimatedDuration(testCases)

	var estimatedDurationStr string
	if estimatedDuration > 0 {
		estimatedDurationStr = formatDuration(estimatedDuration)
	} else {
		estimatedDurationStr = "无历史数据"
	}

	rounds := cfg.Test.Rounds
	if opts.Rounds > 0 {
		rounds = opts.Rounds
	}
	if rounds < 1 {
		rounds = 1
	}

	intervalMs := cfg.Test.IntervalMs
	if opts.IntervalMs > 0 {
		intervalMs = opts.IntervalMs
	}

	executedCount, executedChainCount, executedIndependentCount := testcase.CountTestSummary(testCases)

	fmt.Print(reporting.TestStatsBanner(
		totalTestCaseCount, totalChainCount, totalIndependentCount,
		tags, executedCount, executedChainCount, executedIndependentCount,
		estimatedDurationStr, rounds, intervalMs,
	))

	go func() {
		if err := reporting.SendTestStartEmail(executedCount, executedChainCount, executedIndependentCount, estimatedDuration, rounds, intervalMs); err != nil {
			logger.Warn("发送测试开始通知邮件失败", zap.Error(err))
		}
	}()

	if len(cfg.Test.GlobalPre) > 0 {
		fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
		fmt.Printf("║ 执行全局前置条件                                       ║\n")
		fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")

		for _, preID := range cfg.Test.GlobalPre {
			found := false
			for _, tc := range testCases {
				if tc.ID == preID {
					fmt.Printf("[全局前置] 执行: %s - %s\n", tc.ID, tc.Desc)
					result := testcase.ExecuteTestCaseWithContext(ctx, tc)
					if !result.Passed {
						fmt.Printf("[全局前置] ❌ 失败: %s\n", result.Error)
						fmt.Printf("\n全局前置条件失败，终止测试执行\n")
						errorMsg := fmt.Sprintf("全局前置条件 '%s' 执行失败: %s", tc.ID, result.Error)
						if err := reporting.SendErrorReportEmail(errorMsg); err != nil {
							logger.Warn("发送错误报告邮件失败", zap.Error(err))
						}
						os.Exit(1)
					}
					return nil
				}
				fmt.Printf("[全局前置] ✅ 成功\n")
				found = true
				break
			}
			if !found {
				fmt.Printf("[全局前置] ⚠️ 未找到测试用例: %s\n", preID)
			}
		}
		fmt.Println()
	}

	var allRoundResults []testcase.TestResult
	for round := 1; round <= rounds; round++ {
		if ctx.Err() != nil {
			logger.Info("测试执行被取消，停止后续轮次", zap.Error(ctx.Err()))
			break
		}

		if rounds > 1 {
			fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
			fmt.Printf("║                    第 %d/%d 轮测试                      ║\n", round, rounds)
			fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")
		}

		reportTimestamp := timeutil.FormatCompact(timeutil.Now())
		roundResults := executeTestRound(ctx, testCases, reportTimestamp, round, rounds)
		allRoundResults = append(allRoundResults, roundResults...)

		if round < rounds && intervalMs > 0 {
			fmt.Printf("\n等待 %dms 后开始下一轮测试...\n", intervalMs)
			select {
			case <-time.After(time.Duration(intervalMs) * time.Millisecond):
			case <-ctx.Done():
				logger.Info("轮间等待被取消", zap.Error(ctx.Err()))
				return allRoundResults
			}
		}
	}

	if rounds > 1 {
		fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
		fmt.Printf("║              多轮测试汇总结果                          ║\n")
		fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")
		testcase.PrintSummary(allRoundResults)
	}

	if err := storage.CalculateAndStoreAverages(); err != nil {
		logger.Warn("计算并存储平均耗时失败", zap.Error(err))
	} else {
		logger.Info("成功计算并存储平均耗时")
	}

	if len(cfg.Test.GlobalPost) > 0 {
		fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
		fmt.Printf("║ 执行全局后置条件                                       ║\n")
		fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")

		for _, postID := range cfg.Test.GlobalPost {
			found := false
			for _, tc := range testCases {
				if tc.ID == postID {
					fmt.Printf("[全局后置] 执行: %s - %s\n", tc.ID, tc.Desc)
					result := testcase.ExecuteTestCaseWithContext(ctx, tc)
					if !result.Passed {
						fmt.Printf("[全局后置] ❌ 失败: %s\n", result.Error)
					} else {
						fmt.Printf("[全局后置] ✅ 成功\n")
					}
					found = true
					break
				}
			}
			if !found {
				fmt.Printf("[全局后置] ⚠️ 未找到测试用例: %s\n", postID)
			}
		}
		fmt.Println()
	}

	failedCount := 0
	passedCount := 0
	skippedCount := 0
	var totalDuration time.Duration
	for _, r := range allRoundResults {
		if !r.Passed && !r.TestCase.Skip {
			failedCount++
		} else if r.Passed && !r.TestCase.Skip {
			passedCount++
		} else if r.TestCase.Skip {
			skippedCount++
		}
		totalDuration += r.Duration
	}

	reportingCfg := cfg.Reporting
	if reportingCfg.DailySummary {
		todayStr := timeutil.Now().Format("2006-01-02")
		if err := storage.RecordDailySummary(todayStr, passedCount+failedCount+skippedCount, passedCount, failedCount, skippedCount, totalDuration); err != nil {
			logger.Warn("记录每日汇总失败", zap.Error(err))
		} else {
			logger.Info("每日汇总已记录", zap.String("date", todayStr))
		}
	}

	topSlowN := reportingCfg.TopSlowN
	if topSlowN <= 0 {
		topSlowN = 10
	}
	slowCases, err := storage.GetCaseAverageDurations("desc")
	if err != nil {
		logger.Warn("获取慢接口排行失败，报告将不包含慢接口数据", zap.Error(err))
	} else if len(slowCases) > topSlowN {
		slowCases = slowCases[:topSlowN]
	}
	if len(slowCases) > 0 {
		fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
		fmt.Printf("║              最慢接口 TOP %d                             ║\n", len(slowCases))
		fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")
		fmt.Print(reporting.FormatCaseDurationTable(slowCases))
		fmt.Println()
	}

	consecutiveFailN := reportingCfg.ConsecutiveFailN
	if consecutiveFailN <= 0 {
		consecutiveFailN = 3
	}
	alerts, err := storage.GetConsecutiveFailures(consecutiveFailN)
	if err != nil {
		logger.Warn("获取连续失败告警失败，报告将不包含告警信息",
			zap.Int("consecutive_fail_n", consecutiveFailN),
			zap.Error(err))
	} else if len(alerts) > 0 {
		fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
		fmt.Printf("║  ⚠️  连续失败告警 (连续%d轮)                            ║\n", consecutiveFailN)
		fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")
		for _, a := range alerts {
			fmt.Printf("  ❌ %s - %s (最近执行: %s)\n", a.TestCaseID, a.TestCaseDesc, a.LastExecuted)
		}
		fmt.Println()
	}

	if err := reporting.SendTestReportEmailWithAlerts(allRoundResults, consecutiveFailN, topSlowN); err != nil {
		logger.Warn("发送 HTML 邮件报告失败", zap.Error(err))
	}

	return allRoundResults
}

func calculateEstimatedDuration(testCases []psv.TestCase) time.Duration {
	averages, err := storage.GetAllAverageDurations()
	if err != nil {
		logger.Warn("获取平均耗时失败", zap.Error(err))
		return 0
	}

	if len(averages) == 0 {
		return 0
	}

	var total time.Duration
	unknownCount := 0

	for _, tc := range testCases {
		if tc.Skip {
			continue
		}

		url := vars.Replace(tc.URL)
		if avg, ok := averages[url]; ok {
			total += avg
		} else {
			unknownCount++
		}
	}

	if unknownCount > 0 && len(averages) > 0 {
		var avgAll time.Duration
		for _, avg := range averages {
			avgAll += avg
		}
		avgAll /= time.Duration(len(averages))
		total += avgAll * time.Duration(unknownCount)
	}

	return total
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	} else if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	} else if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	} else {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
}

func InitDirectories() {
	directories := []string{
		"./config",
		"./logs",
		"./reports",
		"./sql",
		"./testcases",
	}

	for _, dir := range directories {
		if dir == "." || dir == "/" {
			continue
		}

		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("警告: 创建目录失败 '%s': %v\n", dir, err)
		}
	}

	createDefaultConfigFile()
}

func executeTestRound(ctx context.Context, testCases []psv.TestCase, reportTimestamp string, round, totalRounds int) []testcase.TestResult {
	var results []testcase.TestResult
	chainFiles := testcase.GetChainFiles(testCases)

	roundStartTime := timeutil.Now()

	for i, tc := range testCases {
		if ctx.Err() != nil {
			logger.Info("测试执行被取消，停止当前轮次", zap.String("case_id", tc.ID), zap.Error(ctx.Err()))
			break
		}
		result := testcase.ExecuteTestCaseWithContext(ctx, tc)
		results = append(results, result)

		fmt.Printf("\n\n────────────────────────────────────────────────────────────\n")
		stepLabel := "测试"
		if chainFiles[tc.FileName] {
			stepLabel = "链式步骤"
		}
		if totalRounds > 1 {
			fmt.Printf("第 %d/%d 轮 - 第 %d/%d 个%s完成，正在更新报告...\n", round, totalRounds, i+1, len(testCases), stepLabel)
		} else {
			fmt.Printf("第 %d/%d 个%s完成，正在更新报告...\n", i+1, len(testCases), stepLabel)
		}
		fmt.Printf("────────────────────────────────────────────────────────────\n")

		allReport, errorReport := testcase.GenerateReport(results)
		allPath, errorPath := testcase.SaveReports(allReport, errorReport, reportTimestamp)

		fmt.Printf("\nPSV 报告已保存: %s\n", allPath)
		if errorPath != "" {
			fmt.Printf("异常用例 PSV 报告已保存: %s\n", errorPath)
		}
	}

	roundTotalDuration := time.Since(roundStartTime)
	correctAndRecordResults(results, roundTotalDuration)

	if totalRounds > 1 {
		fmt.Printf("\n────────────────────────────────────────────────────────────\n")
		fmt.Printf("第 %d/%d 轮测试完成\n", round, totalRounds)
		fmt.Printf("────────────────────────────────────────────────────────────\n")
		testcase.PrintSummary(results)
	}

	return results
}

func correctAndRecordResults(results []testcase.TestResult, totalDuration time.Duration) {
	var actualSum time.Duration
	var passedCount int
	for _, result := range results {
		if result.Passed && !result.TestCase.Skip {
			actualSum += result.Duration
			passedCount++
		}
	}

	var overheadPerTest time.Duration
	if passedCount > 0 {
		overhead := totalDuration - actualSum
		overheadPerTest = overhead / time.Duration(passedCount)

		logger.Info("正在修正执行时间",
			zap.Duration("total_duration", totalDuration),
			zap.Duration("actual_sum", actualSum),
			zap.Duration("overhead", overhead),
			zap.Int("passed_count", passedCount),
			zap.Duration("overhead_per_test", overheadPerTest))
	}

	for _, result := range results {
		if result.TestCase.Skip {
			continue
		}

		duration := result.Duration
		if result.Passed && passedCount > 0 {
			duration += overheadPerTest
		}

		go storage.RecordExecutionTime(
			result.TestCase.ID,
			result.TestCase.Desc,
			result.TestCase.FileName,
			vars.Replace(result.TestCase.URL),
			duration,
			result.Passed,
		)

		logger.Info("已记录执行结果",
			zap.String("test_case_id", result.TestCase.ID),
			zap.Bool("passed", result.Passed),
			zap.Duration("duration", duration))
	}
}

func createDefaultConfigFile() {
	configPath := "./config/config.yaml"

	if _, err := os.Stat(configPath); err == nil {
		return
	}

	defaultConfig := `target:
  base_url: "https://localhost:8080"
  timeout: 30

log:
  level: "info"
  encoding: "json"
  output: "./logs/pipet.log"

test:
  report_dir: "./reports"
  test_case_dir:
    - "./testcases"
  data_dir: "./sql"
  severe_status:
    - 500
  global_pre: []
  global_post: []
  device_name: ""
  rounds: 1
  interval_ms: 0

http:
  insecure_skip_verify: false

vars: {}

email:
  enabled: false
  from: ""
  to: []
  auth_code: ""
  smtp_server: "smtp.example.com"
  smtp_port: 465

cleaner:
  enabled: true
  retention_days: 30
  log_dir: "./logs"
  report_dir: "./reports"
  data_dir: "./sql"
  include_patterns:
    - "*.log"
    - "*.json"
    - "*.csv"
    - "*.txt"
  exclude_patterns: []
  interval_hours: 24

reporting:
  consecutive_fail_n: 3
  top_slow_n: 10
  weekly_enabled: true
  monthly_enabled: true
  yearly_enabled: true
  daily_enabled: true
  daily_summary: true
  send_time: "05:00"
`

	if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
		fmt.Printf("警告: 创建默认配置文件失败 '%s': %v\n", configPath, err)
	} else {
		fmt.Printf("已创建默认配置文件: %s\n", configPath)
	}
}
