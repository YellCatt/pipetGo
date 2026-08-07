// Package cmd 提供命令行接口功能
// 使用 cobra 框架实现命令行参数解析和子命令管理
package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

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
)

var (
	// rootCmd 是命令行应用的根命令
	rootCmd = &cobra.Command{
		Use:   "pipet [paths...]",
		Short: "pipet - API Testing Tool",
		Long:  `A powerful enterprise-grade API testing tool written in Go.`,
		Args:  cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("[DEBUG] rootCmd.Run 被调用, args=%v, sendWeekly=%v, sendMonthly=%v, sendYearly=%v, sendDaily=%v\n",
				args, sendWeeklyFlag, sendMonthlyFlag, sendYearlyFlag, sendDailyFlag)
			if sendWeeklyFlag || sendMonthlyFlag || sendYearlyFlag || sendDailyFlag {
				fmt.Println("[DEBUG] 进入 runSendReports 分支")
				runSendReports()
			} else {
				fmt.Println("[DEBUG] 进入 runTests 分支")
				runTests(args)
			}
			fmt.Println("[DEBUG] rootCmd.Run 执行完毕")
		},
	}

	// tagsFlag 存储命令行指定的标签过滤参数
	tagsFlag string

	// roundsFlag 存储命令行指定的多轮测试次数
	roundsFlag int

	// intervalMsFlag 存储命令行指定的轮间间隔时间（毫秒）
	intervalMsFlag int

	// 报告发送命令行标志
	sendWeeklyFlag  bool
	sendMonthlyFlag bool
	sendYearlyFlag  bool
	sendDailyFlag   bool
)

// Execute 启动命令行应用
// 调用 cobra 的 Execute 方法处理命令行输入
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logger.Error("命令执行失败", zap.Error(err))
		errorMsg := fmt.Sprintf("命令执行失败: %v", err)
		emailCfg := email.GetConfig()
		if emailCfg.Enabled && emailCfg.FromEmail != "" && len(emailCfg.ToEmail) > 0 {
			if sendErr := email.SendErrorReportEmail(errorMsg); sendErr != nil {
				logger.Warn("发送错误报告邮件失败", zap.Error(sendErr))
			}
		}
		os.Exit(1)
	}
}

// init 函数在包初始化时执行
// 注册初始化函数和命令行参数
func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.Flags().StringVar(&config.CfgFile, "config", "", "config file (default is ./config/config.yaml)")
	rootCmd.Flags().StringVarP(&tagsFlag, "tags", "t", "", "filter tests by tags (comma-separated)")
	rootCmd.Flags().IntVarP(&roundsFlag, "rounds", "r", 0, "number of test rounds (default from config)")
	rootCmd.Flags().IntVarP(&intervalMsFlag, "interval", "i", 0, "interval between rounds in milliseconds (default from config)")

	rootCmd.Flags().BoolVar(&sendWeeklyFlag, "send-weekly", false, "立即发送周报邮件")
	rootCmd.Flags().BoolVar(&sendMonthlyFlag, "send-monthly", false, "立即发送月报邮件")
	rootCmd.Flags().BoolVar(&sendYearlyFlag, "send-yearly", false, "立即发送年报邮件")
	rootCmd.Flags().BoolVar(&sendDailyFlag, "send-daily", false, "立即发送日报邮件")

	rootCmd.AddCommand(reportCmd)
}

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "生成ASCII测试报告（含趋势图、慢接口排名、告警）",
	Long:  `生成包含用例增长趋势图、错误率趋势图、慢接口排名、连续失败告警的ASCII综合报告`,
	Run: func(cmd *cobra.Command, args []string) {
		initConfig()
		initStorage()
		runReport()
	},
}

func initStorage() {
	httpclient.InitClient()
	if err := storage.InitDB(config.GetConfig().Test.DataDir); err != nil {
		logger.Warn("CSV 存储初始化失败", zap.Error(err))
	}
}

func runReport() {
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
	waitForScheduler()
}

// runSendReports 根据命令行标志发送指定的报告邮件
func runSendReports() {
	initConfig()
	initStorage()

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

	if sendWeeklyFlag {
		logger.Info("发送周报...")
		weekTmpl := reporting.NewReportTemplate("周报", "本周",
			reportingCfg.WeekTemplate.ShowSummary,
			reportingCfg.WeekTemplate.ShowGrowth,
			reportingCfg.WeekTemplate.ShowError,
			reportingCfg.WeekTemplate.ShowSlow,
			reportingCfg.WeekTemplate.ShowAlert)
		if err := email.SendWeeklyReportEmailWithTemplate(consecutiveFailN, topSlowN, weekTmpl); err != nil {
			logger.Error("发送周报失败", zap.Error(err))
		} else {
			logger.Info("周报发送成功")
		}
	}

	if sendDailyFlag {
		logger.Info("发送日报...")
		dayTmpl := reporting.NewReportTemplate("日报", "本日",
			reportingCfg.DayTemplate.ShowSummary,
			reportingCfg.DayTemplate.ShowGrowth,
			reportingCfg.DayTemplate.ShowError,
			reportingCfg.DayTemplate.ShowSlow,
			reportingCfg.DayTemplate.ShowAlert)
		if err := email.SendDailyReportEmailWithTemplate(consecutiveFailN, topSlowN, dayTmpl); err != nil {
			logger.Error("发送日报失败", zap.Error(err))
		} else {
			logger.Info("日报发送成功")
		}
	}

	if sendMonthlyFlag {
		logger.Info("发送月报...")
		monthTmpl := reporting.NewReportTemplate("月报", "本月",
			reportingCfg.MonthTemplate.ShowSummary,
			reportingCfg.MonthTemplate.ShowGrowth,
			reportingCfg.MonthTemplate.ShowError,
			reportingCfg.MonthTemplate.ShowSlow,
			reportingCfg.MonthTemplate.ShowAlert)
		if err := email.SendMonthlyReportEmailWithTemplate(consecutiveFailN, topSlowN, monthTmpl); err != nil {
			logger.Error("发送月报失败", zap.Error(err))
		} else {
			logger.Info("月报发送成功")
		}
	}

	if sendYearlyFlag {
		logger.Info("发送年报...")
		yearTmpl := reporting.NewReportTemplate("年报", "本年",
			reportingCfg.YearTemplate.ShowSummary,
			reportingCfg.YearTemplate.ShowGrowth,
			reportingCfg.YearTemplate.ShowError,
			reportingCfg.YearTemplate.ShowSlow,
			reportingCfg.YearTemplate.ShowAlert)
		if err := email.SendYearlyReportEmailWithTemplate(consecutiveFailN, topSlowN, yearTmpl); err != nil {
			logger.Error("发送年报失败", zap.Error(err))
		} else {
			logger.Info("年报发送成功")
		}
	}

	waitForScheduler()
}

// initConfig 初始化应用配置
// 依次初始化：必要目录和默认配置、配置、日志、全局变量、邮件配置
func initConfig() {
	fmt.Println("[DEBUG] initConfig 开始执行")

	// 自动创建必要的目录和默认配置文件
	fmt.Println("[DEBUG] 调用 initDirectories()")
	initDirectories()
	fmt.Println("[DEBUG] initDirectories() 完成")

	fmt.Println("[DEBUG] 调用 config.InitConfig()")
	config.InitConfig()
	fmt.Println("[DEBUG] config.InitConfig() 完成")

	fmt.Printf("[DEBUG] 日志配置: level=%s, encoding=%s, output=%s\n",
		config.AppConfig.Log.Level, config.AppConfig.Log.Encoding, config.AppConfig.Log.Output)

	logger.InitLogger(logger.LogConfig{
		Level:    config.AppConfig.Log.Level,
		Encoding: config.AppConfig.Log.Encoding,
		Output:   config.AppConfig.Log.Output,
	})
	logger.Debug("logger 初始化完成")

	// 初始化内置变量
	vars.Set("base_url", config.AppConfig.Target.BaseURL)
	logger.Debug("内置变量 base_url 已设置", zap.String("base_url", config.AppConfig.Target.BaseURL))

	// 加载用户自定义变量（支持任意变量名）
	if len(config.AppConfig.Vars) > 0 {
		vars.InitFromConfig(config.AppConfig.Vars)
		logger.Info("用户自定义变量加载完成", zap.Int("count", len(config.AppConfig.Vars)), zap.Any("vars", maskVars(config.AppConfig.Vars)))
	} else {
		logger.Info("未配置用户自定义变量")
	}
	logger.Info("当前可用变量", zap.Any("vars", vars.GetAll()))

	logger.Debug("开始初始化邮件配置")
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

	logger.Debug("开始启动调度器")
	scheduler.Start(
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
		func() {
			runTestCycle(nil)
		},
	)
	logger.Debug("调度器启动完成")

	// 注册配置变更回调（热加载时更新各组件）
	config.RegisterOnChange(func(newCfg config.Config) {
		// 更新日志级别
		logger.UpdateLevel(newCfg.Log.Level)

		// 更新邮件配置
		email.UpdateConfig(email.EmailConfig{
			Enabled:    newCfg.Email.Enabled,
			FromEmail:  newCfg.Email.From,
			ToEmail:    newCfg.Email.To,
			AuthCode:   newCfg.Email.AuthCode,
			SMTPServer: newCfg.Email.SMTPServer,
			SMTPPort:   newCfg.Email.SMTPPort,
			DeviceName: newCfg.Test.DeviceName,
		})

		// 更新内置变量
		vars.Set("base_url", newCfg.Target.BaseURL)

		// 更新用户自定义变量
		if len(newCfg.Vars) > 0 {
			vars.InitFromConfig(newCfg.Vars)
		}

		logger.Info("配置已热更新",
			zap.String("log_level", newCfg.Log.Level),
			zap.Bool("email_enabled", newCfg.Email.Enabled),
			zap.String("base_url", newCfg.Target.BaseURL),
		)
	})

	fmt.Println("[DEBUG] initConfig 执行完成")
}
func maskVars(vars map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range vars {
		result[k] = maskString(v)
	}
	return result
}

// maskString 用于日志中掩码敏感信息
func maskString(s string) string {
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "***" + s[len(s)-4:]
}

// runTestCycle 执行一轮完整的测试流程（不阻塞，用于定时调度）
// paths: 用户指定的测试用例路径列表
func runTestCycle(paths []string) {
	fmt.Println("[DEBUG] runTestCycle 开始执行")
	cfg := config.GetConfig()
	fmt.Printf("[DEBUG] 获取配置完成, test_case_dir=%v, data_dir=%s\n", cfg.Test.TestCaseDir, cfg.Test.DataDir)

	// 启动时打印横幅到 stdout，确保手动执行时能看到输出
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════╗")
	fmt.Println("║           pipetGo API 测试工具                          ║")
	fmt.Println("╠════════════════════════════════════════════════════════╣")
	fmt.Printf("║  启动时间: %-43s ║\n", timeutil.FormatDateTime(timeutil.Now()))
	fmt.Printf("║  测试设备: %-43s ║\n", cfg.Test.DeviceName)
	fmt.Printf("║  日志文件: %-43s ║\n", cfg.Log.Output)
	fmt.Println("╚════════════════════════════════════════════════════════╝")
	fmt.Println()

	fmt.Println("[DEBUG] 初始化 HTTP 客户端")
	// 初始化 HTTP 客户端
	httpclient.InitClient()
	fmt.Println("[DEBUG] HTTP 客户端初始化完成")

	// 初始化 CSV 存储
	logger.Info("准备初始化 CSV 存储", zap.String("DataDir", cfg.Test.DataDir))
	if err := storage.InitDB(cfg.Test.DataDir); err != nil {
		logger.Warn("CSV 存储初始化失败", zap.Error(err))
	} else {
		logger.Info("CSV 存储初始化成功")

		// 检查历史记录数
		count, err := storage.GetTotalExecutionCount()
		if err != nil {
			logger.Warn("获取执行计数失败", zap.Error(err))
		} else {
			logger.Info("找到历史执行记录", zap.Int("count", count))
		}
	}

	// 如果未指定路径，使用默认测试用例目录
	if len(paths) == 0 {
		paths = cfg.Test.TestCaseDir
	}
	fmt.Printf("[DEBUG] 待解析的测试用例路径: %v\n", paths)

	// 解析 PSV/CSV 测试用例文件
	fmt.Println("[DEBUG] 开始解析 PSV/CSV 测试用例文件")
	testCases, err := psv.ParseFiles(paths)
	if err != nil {
		logger.Error("解析 PSV 文件失败", zap.Error(err))
		errorMsg := fmt.Sprintf("解析测试用例文件失败: %v", err)
		if err := email.SendErrorReportEmail(errorMsg); err != nil {
			logger.Warn("发送错误报告邮件失败", zap.Error(err))
		}
		os.Exit(1)
	}
	fmt.Printf("[DEBUG] PSV 解析完成, 共解析 %d 个测试用例\n", len(testCases))

	// 设置所有测试用例（用于链式测试查找前置条件）
	testcase.SetAllTestCases(testCases)

	// 解析标签过滤参数
	var tags []string
	if tagsFlag != "" {
		tags = strings.Split(tagsFlag, ",")
		for i, tag := range tags {
			tags[i] = strings.TrimSpace(tag)
		}
	}

	// 保存原始测试用例总数（过滤前），链式文件按 1 个计数
	totalTestCaseCount, totalChainCount, totalIndependentCount := testcase.CountTestSummary(testCases)

	// 根据标签过滤测试用例
	testCases = testcase.FilterByTags(testCases, tags)
	fmt.Printf("[DEBUG] 标签过滤后剩余 %d 个测试用例\n", len(testCases))

	// 如果没有测试用例，直接返回
	if len(testCases) == 0 {
		fmt.Println("[DEBUG] 没有需要执行的测试用例，runTestCycle 返回")
		logger.Info("没有需要执行的测试用例")
		return
	}

	// 开始执行测试
	logger.Info("开始执行 API 测试", zap.Int("count", len(testCases)))

	// 计算预估执行时间
	estimatedDuration := calculateEstimatedDuration(testCases)

	// 格式化预估时间
	var estimatedDurationStr string
	if estimatedDuration > 0 {
		estimatedDurationStr = formatDuration(estimatedDuration)
	} else {
		estimatedDurationStr = "无历史数据"
	}

	// 确定多轮测试配置（命令行参数优先于配置文件）
	rounds := cfg.Test.Rounds
	if roundsFlag > 0 {
		rounds = roundsFlag
	}
	if rounds < 1 {
		rounds = 1
	}

	intervalMs := cfg.Test.IntervalMs
	if intervalMsFlag > 0 {
		intervalMs = intervalMsFlag
	}

	// 打印本次执行的测试用例统计信息
	executedCount, executedChainCount, executedIndependentCount := testcase.CountTestSummary(testCases)

	fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║ 测试用例统计信息                                       ║\n")
	fmt.Printf("╠════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ 解析出的测试用例总数: %-35d║\n", totalTestCaseCount)
	fmt.Printf("║   链式测试: %-43d║\n", totalChainCount)
	fmt.Printf("║   独立测试: %-43d║\n", totalIndependentCount)
	if len(tags) > 0 {
		fmt.Printf("║ 应用标签过滤: %-40s║\n", strings.Join(tags, ", "))
		fmt.Printf("║ 过滤后实际执行数: %-36d║\n", executedCount)
		fmt.Printf("║   链式测试: %-43d║\n", executedChainCount)
		fmt.Printf("║   独立测试: %-43d║\n", executedIndependentCount)
	} else {
		fmt.Printf("║ 未应用标签过滤，本次共执行 %-27d║\n", executedCount)
		fmt.Printf("║   链式测试: %-43d║\n", executedChainCount)
		fmt.Printf("║   独立测试: %-43d║\n", executedIndependentCount)
	}

	fmt.Printf("╠════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ 预估执行时间: %-41s║\n", estimatedDurationStr)
	if rounds > 1 {
		fmt.Printf("║ 多轮测试配置: %d 轮，每轮间隔 %dms                     ║\n", rounds, intervalMs)
	}
	fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")

	// 发送测试开始通知邮件
	go func() {
		if err := email.SendTestStartEmail(executedCount, executedChainCount, executedIndependentCount, estimatedDuration, rounds, intervalMs); err != nil {
			logger.Warn("发送测试开始通知邮件失败", zap.Error(err))
		}
	}()

	// 执行全局前置条件（所有测试用例执行前运行，仅在第一轮执行）
	if len(cfg.Test.GlobalPre) > 0 {
		fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
		fmt.Printf("║ 执行全局前置条件                                       ║\n")
		fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")

		for _, preID := range cfg.Test.GlobalPre {
			found := false
			for _, tc := range testCases {
				if tc.ID == preID {
					fmt.Printf("[全局前置] 执行: %s - %s\n", tc.ID, tc.Desc)
					result := testcase.ExecuteTestCase(tc)
					if !result.Passed {
						fmt.Printf("[全局前置] ❌ 失败: %s\n", result.Error)
						fmt.Printf("\n全局前置条件失败，终止测试执行\n")
						errorMsg := fmt.Sprintf("全局前置条件 '%s' 执行失败: %s", tc.ID, result.Error)
						if err := email.SendErrorReportEmail(errorMsg); err != nil {
							logger.Warn("发送错误报告邮件失败", zap.Error(err))
						}
						os.Exit(1)
					}
					fmt.Printf("[全局前置] ✅ 成功\n")
					found = true
					break
				}
			}
			if !found {
				fmt.Printf("[全局前置] ⚠️ 未找到测试用例: %s\n", preID)
			}
		}
		fmt.Println()
	}

	// 多轮测试执行
	var allRoundResults []testcase.TestResult
	for round := 1; round <= rounds; round++ {
		if rounds > 1 {
			fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
			fmt.Printf("║                    第 %d/%d 轮测试                      ║\n", round, rounds)
			fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")
		}

		// 生成报告时间戳（每轮使用不同的时间戳）
		reportTimestamp := timeutil.FormatCompact(timeutil.Now())

		// 执行本轮测试
		roundResults := executeTestRound(testCases, reportTimestamp, round, rounds)
		allRoundResults = append(allRoundResults, roundResults...)

		// 轮间等待（最后一轮不需要等待）
		if round < rounds && intervalMs > 0 {
			fmt.Printf("\n等待 %dms 后开始下一轮测试...\n", intervalMs)
			time.Sleep(time.Duration(intervalMs) * time.Millisecond)
		}
	}

	// 打印多轮测试汇总摘要
	if rounds > 1 {
		fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
		fmt.Printf("║              多轮测试汇总结果                          ║\n")
		fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")
		testcase.PrintSummary(allRoundResults)
	}

	// 测试结束后计算并存储所有成功测试用例的平均执行时间
	if err := storage.CalculateAndStoreAverages(); err != nil {
		logger.Warn("计算并存储平均耗时失败", zap.Error(err))
	} else {
		logger.Info("成功计算并存储平均耗时")
	}

	// 执行全局后置条件（所有测试用例执行后运行）
	if len(cfg.Test.GlobalPost) > 0 {
		fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
		fmt.Printf("║ 执行全局后置条件                                       ║\n")
		fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")

		for _, postID := range cfg.Test.GlobalPost {
			found := false
			for _, tc := range testCases {
				if tc.ID == postID {
					fmt.Printf("[全局后置] 执行: %s - %s\n", tc.ID, tc.Desc)
					result := testcase.ExecuteTestCase(tc)
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

	// 如果有失败的测试用例，退出码设为 1
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

	// 记录每日汇总
	reportingCfg := cfg.Reporting
	if reportingCfg.DailySummary {
		todayStr := timeutil.Now().Format("2006-01-02")
		if err := storage.RecordDailySummary(todayStr, passedCount+failedCount+skippedCount, passedCount, failedCount, skippedCount, totalDuration); err != nil {
			logger.Warn("记录每日汇总失败", zap.Error(err))
		} else {
			logger.Info("每日汇总已记录", zap.String("date", todayStr))
		}
	}

	// 打印慢接口排名
	topSlowN := reportingCfg.TopSlowN
	if topSlowN <= 0 {
		topSlowN = 10
	}
	slowCases, _ := storage.GetCaseAverageDurations("desc")
	if len(slowCases) > topSlowN {
		slowCases = slowCases[:topSlowN]
	}
	if len(slowCases) > 0 {
		fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
		fmt.Printf("║              最慢接口 TOP %d                             ║\n", len(slowCases))
		fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")
		fmt.Printf(reporting.FormatCaseDurationTable(slowCases))
		fmt.Println()
	}

	// 连续失败告警检查
	consecutiveFailN := reportingCfg.ConsecutiveFailN
	if consecutiveFailN <= 0 {
		consecutiveFailN = 3
	}
	alerts, _ := storage.GetConsecutiveFailures(consecutiveFailN)
	if len(alerts) > 0 {
		fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
		fmt.Printf("║  ⚠️  连续失败告警 (连续%d轮)                            ║\n", consecutiveFailN)
		fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")
		for _, a := range alerts {
			fmt.Printf("  ❌ %s - %s (最近执行: %s)\n", a.TestCaseID, a.TestCaseDesc, a.LastExecuted)
		}
		fmt.Println()
	}

	// 测试结束后发送HTML邮件报告（含连续失败标红告警）
	if err := email.SendTestReportEmailWithAlerts(allRoundResults, consecutiveFailN, topSlowN); err != nil {
		logger.Warn("发送 HTML 邮件报告失败", zap.Error(err))
	}
}

// runTests 执行测试主流程，完成后进入 daemon 等待
func runTests(paths []string) {
	fmt.Printf("[DEBUG] runTests 被调用, paths=%v\n", paths)
	logger.Debug("runTests 开始执行", zap.Strings("paths", paths))
	runTestCycle(paths)
	fmt.Println("[DEBUG] runTestCycle 执行完成")
	waitForScheduler()
	fmt.Println("[DEBUG] waitForScheduler 返回")
}

// waitForScheduler 如果调度器正在运行，则阻塞保持进程存活（daemon 模式）
func waitForScheduler() {
	if !scheduler.IsRunning() {
		return
	}
	logger.Info("守护进程模式: 调度器正在运行，保持进程存活...")
	select {}
}

// calculateEstimatedDuration 根据历史执行时间计算预估总耗时
func calculateEstimatedDuration(testCases []psv.TestCase) time.Duration {
	// 获取所有 URL 的平均执行时间
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
		// 跳过被标记为跳过的测试用例
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

	// 如果有未知 URL，使用已知 URL 的平均时间作为估算
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

// formatDuration 格式化时间为可读字符串
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

// initDirectories 自动创建必要的目录和默认配置文件
// 如果目录不存在则创建，已存在则跳过
func initDirectories() {
	fmt.Println("[DEBUG] initDirectories 开始执行")
	// 需要创建的目录列表（使用默认值，因为此时配置还未加载）
	directories := []string{
		"./config",    // 配置文件目录
		"./logs",      // 日志目录
		"./reports",   // 测试报告目录
		"./sql",       // 数据存储目录（CSV文件）
		"./testcases", // 测试用例目录
	}

	for _, dir := range directories {
		if dir == "." || dir == "/" {
			continue
		}

		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("警告: 创建目录失败 '%s': %v\n", dir, err)
		}
	}

	// 检查并创建默认配置文件
	createDefaultConfigFile()
	fmt.Println("[DEBUG] initDirectories 执行完成")
}

// executeTestRound 执行单轮测试
// testCases: 测试用例列表
// reportTimestamp: 报告时间戳
// round: 当前轮次
// totalRounds: 总轮数
// 返回: 本轮测试结果
func executeTestRound(testCases []psv.TestCase, reportTimestamp string, round, totalRounds int) []testcase.TestResult {
	var results []testcase.TestResult
	chainFiles := testcase.GetChainFiles(testCases)

	// 记录轮次开始时间，用于计算总耗时
	roundStartTime := timeutil.Now()

	for i, tc := range testCases {
		result := testcase.ExecuteTestCase(tc)
		results = append(results, result)

		// 每完成一个测试用例就更新一次报告（覆盖同一个文件）
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

		// 生成并保存测试报告（使用同一个时间戳，覆盖之前的报告）
		allReport, errorReport := testcase.GenerateReport(results)
		allPath, errorPath := testcase.SaveReports(allReport, errorReport, reportTimestamp)

		// 输出报告路径
		fmt.Printf("\nPSV 报告已保存: %s\n", allPath)
		if errorPath != "" {
			fmt.Printf("异常用例 PSV 报告已保存: %s\n", errorPath)
		}
	}

	// 计算轮次总耗时
	roundTotalDuration := time.Since(roundStartTime)

	// 计算修正后的执行时间，将系统开销平均分摊到每个成功的测试用例
	correctAndRecordResults(results, roundTotalDuration)

	// 打印本轮测试摘要（多轮模式下）
	if totalRounds > 1 {
		fmt.Printf("\n────────────────────────────────────────────────────────────\n")
		fmt.Printf("第 %d/%d 轮测试完成\n", round, totalRounds)
		fmt.Printf("────────────────────────────────────────────────────────────\n")
		testcase.PrintSummary(results)
	}

	return results
}

// correctAndRecordResults 修正测试结果的执行时间并记录到存储
// 将总耗时与各测试用例实际时长之和的差值平均分摊到每个成功的测试用例
func correctAndRecordResults(results []testcase.TestResult, totalDuration time.Duration) {
	// 计算所有成功测试用例的实际时长之和
	var actualSum time.Duration
	var passedCount int
	for _, result := range results {
		if result.Passed && !result.TestCase.Skip {
			actualSum += result.Duration
			passedCount++
		}
	}

	// 如果没有成功的测试用例，无需修正
	if passedCount == 0 {
		return
	}

	// 计算系统开销（总时长与实际时长之和的差值）
	overhead := totalDuration - actualSum

	// 计算每个成功测试用例需要分摊的开销
	overheadPerTest := overhead / time.Duration(passedCount)

	logger.Info("正在修正执行时间",
		zap.Duration("total_duration", totalDuration),
		zap.Duration("actual_sum", actualSum),
		zap.Duration("overhead", overhead),
		zap.Int("passed_count", passedCount),
		zap.Duration("overhead_per_test", overheadPerTest))

	// 修正每个成功测试用例的执行时间并记录到存储
	for _, result := range results {
		if result.Passed && !result.TestCase.Skip {
			// 计算修正后的执行时间
			correctedDuration := result.Duration + overheadPerTest

			// 异步记录修正后的执行时间到存储
			go storage.RecordExecutionTime(
				result.TestCase.ID,
				result.TestCase.Desc,
				result.TestCase.FileName,
				vars.Replace(result.TestCase.URL),
				correctedDuration,
				true,
			)

			logger.Info("已记录修正后的执行时间",
				zap.String("test_case_id", result.TestCase.ID),
				zap.Duration("original_duration", result.Duration),
				zap.Duration("corrected_duration", correctedDuration))
		}
	}
}

// createDefaultConfigFile 如果 config.yaml 不存在，创建默认配置文件
func createDefaultConfigFile() {
	configPath := "./config/config.yaml"

	// 检查文件是否存在
	if _, err := os.Stat(configPath); err == nil {
		// 文件已存在，跳过创建
		return
	}

	// 默认配置内容
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