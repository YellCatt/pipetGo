package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"pipetGo/config"
	"pipetGo/internal/app"
	"pipetGo/internal/logger"
	"pipetGo/internal/reporting"
)

var (
	rootCmd = &cobra.Command{
		Use:   "pipet [paths...]",
		Short: "pipet - API 测试工具",
		Long:  `一款基于 Go 语言开发的企业级 API 测试工具。`,
		Args:  cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			logger.Debug("根命令被调用",
				zap.Strings("参数", args),
				zap.Bool("发送周报", sendWeeklyFlag),
				zap.Bool("发送月报", sendMonthlyFlag),
				zap.Bool("发送年报", sendYearlyFlag),
				zap.Bool("发送日报", sendDailyFlag))

			ctx := cmd.Context()
			app.Init(ctx)
			app.InitStorage()

			opts := app.Options{
				Tags:        tagsFlag,
				Rounds:      roundsFlag,
				IntervalMs:  intervalMsFlag,
				SendWeekly:  sendWeeklyFlag,
				SendMonthly: sendMonthlyFlag,
				SendYearly:  sendYearlyFlag,
				SendDaily:   sendDailyFlag,
			}

			if sendWeeklyFlag || sendMonthlyFlag || sendYearlyFlag || sendDailyFlag {
				logger.Debug("进入报表发送分支")
				app.RunSendReports(ctx, opts)
			} else {
				logger.Debug("进入测试执行分支")
				app.RunTests(ctx, args, opts)
			}
			logger.Debug("根命令执行完毕")
		},
	}

	tagsFlag       string
	roundsFlag     int
	intervalMsFlag int

	sendWeeklyFlag  bool
	sendMonthlyFlag bool
	sendYearlyFlag  bool
	sendDailyFlag   bool
)

func ExecuteContext(ctx context.Context) {
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		logger.Error("命令执行失败", zap.Error(err))
		errorMsg := fmt.Sprintf("命令执行失败: %v", err)
		if err := reporting.SendErrorReportEmail(errorMsg); err != nil {
			logger.Warn("发送错误报告邮件失败", zap.Error(err))
		}
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringVar(&config.CfgFile, "config", "", "配置文件路径 (默认: ./config/config.yaml)")
	rootCmd.Flags().StringVarP(&tagsFlag, "tags", "t", "", "按标签过滤测试用例 (多个标签用逗号分隔)")
	rootCmd.Flags().IntVarP(&roundsFlag, "rounds", "r", 0, "测试轮数 (默认使用配置文件)")
	rootCmd.Flags().IntVarP(&intervalMsFlag, "interval", "i", 0, "轮间间隔毫秒数 (默认使用配置文件)")

	rootCmd.Flags().BoolVar(&sendWeeklyFlag, "send-weekly", false, "立即发送周报邮件")
	rootCmd.Flags().BoolVar(&sendMonthlyFlag, "send-monthly", false, "立即发送月报邮件")
	rootCmd.Flags().BoolVar(&sendYearlyFlag, "send-yearly", false, "立即发送年报邮件")
	rootCmd.Flags().BoolVar(&sendDailyFlag, "send-daily", false, "立即发送日报邮件")

	rootCmd.AddCommand(reportCmd)
}

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "生成 ASCII 测试报告（含趋势图、慢接口排名、告警）",
	Long:  `生成包含用例增长趋势图、错误率趋势图、慢接口排名、连续失败告警的 ASCII 综合报告`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()
		app.Init(ctx)
		app.InitStorage()
		app.RunReport(ctx)
	},
}