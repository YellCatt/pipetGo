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
			runTests(args)
		},
	}

	// tagsFlag 存储命令行指定的标签过滤参数
	tagsFlag string
)

// Execute 启动命令行应用
// 调用 cobra 的 Execute 方法处理命令行输入
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logger.Error("Failed to execute command", zap.Error(err))
		os.Exit(1)
	}
}

// init 函数在包初始化时执行
// 注册初始化函数和命令行参数
func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.Flags().StringVar(&config.CfgFile, "config", "", "config file (default is ./config/config.yaml)")
	rootCmd.Flags().StringVarP(&tagsFlag, "tags", "t", "", "filter tests by tags (comma-separated)")
}

// initConfig 初始化应用配置
// 依次初始化：配置、日志、全局变量、邮件配置
func initConfig() {
	config.InitConfig()
	logger.InitLogger(logger.LogConfig{
		Level:    config.AppConfig.Log.Level,
		Encoding: config.AppConfig.Log.Encoding,
		Output:   config.AppConfig.Log.Output,
	})
	vars.Set("base_url", config.AppConfig.Target.BaseURL)

	email.InitEmail(email.EmailConfig{
		FromEmail:  config.AppConfig.Email.From,
		ToEmail:    config.AppConfig.Email.To,
		AuthCode:   config.AppConfig.Email.AuthCode,
		SMTPServer: config.AppConfig.Email.SMTPServer,
		SMTPPort:   config.AppConfig.Email.SMTPPort,
	})
}

// runTests 执行测试主流程
// paths: 用户指定的测试用例路径列表
func runTests(paths []string) {
	// 初始化 HTTP 客户端
	httpclient.InitClient()

	// 初始化 SQLite 数据库
	logger.Info("准备初始化 SQLite 数据库", zap.String("DataDir", config.AppConfig.Test.DataDir))
	if err := storage.InitDB(config.AppConfig.Test.DataDir); err != nil {
		logger.Warn("SQLite 数据库初始化失败", zap.Error(err))
	} else {
		logger.Info("SQLite 数据库初始化成功")
	}

	// 如果未指定路径，使用默认测试用例目录
	if len(paths) == 0 {
		paths = []string{config.AppConfig.Test.TestCaseDir}
	}

	// 解析 PSV/CSV 测试用例文件
	testCases, err := psv.ParseFiles(paths)
	if err != nil {
		logger.Error("Failed to parse PSV files", zap.Error(err))
		os.Exit(1)
	}

	// 解析标签过滤参数
	var tags []string
	if tagsFlag != "" {
		tags = strings.Split(tagsFlag, ",")
		for i, tag := range tags {
			tags[i] = strings.TrimSpace(tag)
		}
	}

	// 保存原始测试用例总数（过滤前）
	totalTestCaseCount := len(testCases)

	// 根据标签过滤测试用例
	testCases = testcase.FilterByTags(testCases, tags)

	// 如果没有测试用例，直接返回
	if len(testCases) == 0 {
		logger.Info("No test cases to run")
		return
	}

	// 开始执行测试
	logger.Info("Starting API tests", zap.Int("count", len(testCases)))
	
	// 计算预估执行时间
	estimatedDuration := calculateEstimatedDuration(testCases)

	// 格式化预估时间
	var estimatedDurationStr string
	if estimatedDuration > 0 {
		estimatedDurationStr = formatDuration(estimatedDuration)
	} else {
		estimatedDurationStr = "无历史数据"
	}

	// 打印本次执行的测试用例统计信息
	fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║ 测试用例统计信息                                       ║\n")
	fmt.Printf("╠════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ 解析出的测试用例总数: %d                                ║\n", totalTestCaseCount)
	if len(tags) > 0 {
		fmt.Printf("║ 应用标签过滤: %s                                       ║\n", strings.Join(tags, ", "))
		fmt.Printf("║ 过滤后实际执行数: %d                                   ║\n", len(testCases))
	} else {
		fmt.Printf("║ 未应用标签过滤，本次共执行 %d 个测试用例                  ║\n", len(testCases))
	}
	fmt.Printf("╠════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ 预估执行时间: %s                                        ║\n", estimatedDurationStr)
	fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")

	// 发送测试开始通知邮件
	go func() {
		if err := email.SendTestStartEmail(len(testCases), estimatedDurationStr); err != nil {
			logger.Warn("Failed to send test start email", zap.Error(err))
		}
	}()

	// 生成报告时间戳（测试开始时生成，后续更新报告时使用同一个时间戳）
	reportTimestamp := timeutil.FormatCompact(timeutil.Now())

	// 运行测试（串行执行），每完成一个测试就更新一次报告
	var results []testcase.TestResult
	for i, tc := range testCases {
		result := testcase.ExecuteTestCase(tc)
		results = append(results, result)

		// 每完成一个测试用例就更新一次报告（覆盖同一个文件）
		fmt.Printf("\n\n────────────────────────────────────────────────────────────\n")
		fmt.Printf("第 %d/%d 个测试完成，正在更新报告...\n", i+1, len(testCases))
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

	// 打印最终测试摘要
	testcase.PrintSummary(results)

	// 测试结束后计算并存储所有成功测试用例的平均执行时间
	if err := storage.CalculateAndStoreAverages(); err != nil {
		logger.Warn("Failed to calculate and store average durations", zap.Error(err))
	} else {
		logger.Info("Successfully calculated and stored average durations")
	}

	// 如果有失败的测试用例，退出码设为 1
	failedCount := 0
	for _, r := range results {
		if !r.Passed && !r.TestCase.Skip {
			failedCount++
		}
	}

	// 测试结束后发送邮件报告
	if err := email.SendTestReportEmail(results); err != nil {
		logger.Warn("Failed to send email report", zap.Error(err))
	}

	if failedCount > 0 {
		os.Exit(1)
	}
}

// calculateEstimatedDuration 根据历史执行时间计算预估总耗时
func calculateEstimatedDuration(testCases []psv.TestCase) time.Duration {
	// 获取所有 URL 的平均执行时间
	averages, err := storage.GetAllAverageDurations()
	if err != nil {
		logger.Warn("Failed to get average durations", zap.Error(err))
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