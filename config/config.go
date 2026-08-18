// Package config 提供配置管理功能
// 使用 viper 读取 YAML 配置文件并解析到结构体中
package config

import (
	"fmt"
	"log"
	"os"
	"pipetGo/internal/logger"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// CfgFile 存储命令行指定的配置文件路径
var CfgFile string

// Config 表示应用程序的完整配置
type Config struct {
	Target    TargetConfig      `mapstructure:"target"`    // 目标 API 配置
	Log       LogConfig         `mapstructure:"log"`       // 日志配置
	Test      TestConfig        `mapstructure:"test"`      // 测试配置
	HTTP      HTTPConfig        `mapstructure:"http"`      // HTTP 客户端配置
	Email     EmailConfig       `mapstructure:"email"`     // 邮件配置
	Cleaner   CleanupConfig     `mapstructure:"cleaner"`   // 自动清理配置
	Reporting ReportingConfig   `mapstructure:"reporting"` // 报告和告警配置
	Vars      map[string]string `mapstructure:"vars"`      // 用户自定义变量（用于替换测试用例中的 {{var}}）
}

// HTTPConfig 表示 HTTP 客户端的配置
type HTTPConfig struct {
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify"` // 是否跳过 TLS 证书验证
}

// TargetConfig 表示目标 API 的配置
type TargetConfig struct {
	BaseURL       string `mapstructure:"base_url"`      // API 基础地址
	Timeout       int    `mapstructure:"timeout"`       // 请求超时时间（秒）
	Authorization string `mapstructure:"authorization"` // API 授权令牌
	UserId        string `mapstructure:"user_id"`       // 用户 ID
}

// LogConfig 表示日志系统的配置
type LogConfig struct {
	Level    string `mapstructure:"level"`    // 日志级别 (debug/info/warn/error)
	Encoding string `mapstructure:"encoding"` // 日志格式 (json/console)
	Output   string `mapstructure:"output"`   // 输出位置 (stdout 或文件路径)
}

// TestConfig 表示测试相关的配置
type TestConfig struct {
	ReportDir   string   `mapstructure:"report_dir"`    // 测试报告输出目录
	TestCaseDir []string `mapstructure:"test_case_dir"` // 默认测试用例目录（支持多个）
	DataDir     string   `mapstructure:"data_dir"`      // 数据存储目录（用于 CSV 文件）

	SevereStatus            []int    `mapstructure:"severe_status"`             // 严重错误状态码列表，这些状态码的测试用例失败时优先于其他失败用例
	GlobalPre               []string `mapstructure:"global_pre"`                // 全局前置条件测试用例ID列表（所有测试执行前运行）
	GlobalPost              []string `mapstructure:"global_post"`               // 全局后置条件测试用例ID列表（所有测试执行后运行）
	DeviceName              string   `mapstructure:"device_name"`               // 测试设备名称（未配置时自动使用主机名）
	Rounds                  int      `mapstructure:"rounds"`                    // 多轮测试次数，默认为1（单次测试）
	IntervalMs              int      `mapstructure:"interval_ms"`               // 轮间间隔时间（毫秒），默认为0
	ScheduleIntervalMinutes int      `mapstructure:"schedule_interval_minutes"` // 定时执行测试间隔（分钟），0 表示不启用定时执行
}

// EmailConfig 表示邮件发送相关的配置
type EmailConfig struct {
	Enabled    bool     `mapstructure:"enabled"`     // 是否启用邮件发送
	From       string   `mapstructure:"from"`        // 发件人邮箱
	To         []string `mapstructure:"to"`          // 收件人邮箱列表
	AuthCode   string   `mapstructure:"auth_code"`   // 邮箱授权码
	SMTPServer string   `mapstructure:"smtp_server"` // SMTP 服务器地址
	SMTPPort   int      `mapstructure:"smtp_port"`   // SMTP 端口
}

// CleanupConfig 表示自动清理相关的配置
type CleanupConfig struct {
	Enabled         bool     `mapstructure:"enabled"`          // 是否启用自动清理
	RetentionDays   int      `mapstructure:"retention_days"`   // 文件保留天数
	LogDir          string   `mapstructure:"log_dir"`          // 日志目录（自动从 log.output 提取）
	ReportDir       string   `mapstructure:"report_dir"`       // 测试报告目录（自动从 test.report_dir 提取）
	DataDir         string   `mapstructure:"data_dir"`         // 数据目录（自动从 test.data_dir 提取）
	IncludePatterns []string `mapstructure:"include_patterns"` // 要清理的文件模式列表（如 *.log, *.json）
	ExcludePatterns []string `mapstructure:"exclude_patterns"` // 排除的文件模式列表
	IntervalHours   int      `mapstructure:"interval_hours"`   // 定时清理间隔（小时）
}

// ReportingConfig 表示报告和告警相关的配置
type ReportingConfig struct {
	ConsecutiveFailN int                  `mapstructure:"consecutive_fail_n"` // 连续失败N轮告警阈值
	TopSlowN         int                  `mapstructure:"top_slow_n"`         // 慢接口排名 TOP N
	WeeklyEnabled    bool                 `mapstructure:"weekly_enabled"`     // 是否启用周报
	MonthlyEnabled   bool                 `mapstructure:"monthly_enabled"`    // 是否启用月报
	YearlyEnabled    bool                 `mapstructure:"yearly_enabled"`     // 是否启用年报
	DailyEnabled     bool                 `mapstructure:"daily_enabled"`      // 是否启用日报
	DailySummary     bool                 `mapstructure:"daily_summary"`      // 是否启用每日汇总记录
	SendTime         string               `mapstructure:"send_time"`          // 报告发送时间，格式 "HH:MM"，默认 "05:00"
	DayTemplate      ReportTemplateConfig `mapstructure:"day_template"`       // 日报模板配置
	WeekTemplate     ReportTemplateConfig `mapstructure:"week_template"`      // 周报模板配置
	MonthTemplate    ReportTemplateConfig `mapstructure:"month_template"`     // 月报模板配置
	YearTemplate     ReportTemplateConfig `mapstructure:"year_template"`      // 年报模板配置
}

// ReportTemplateConfig 定义报告模板的配置，控制报告中各模块的显示
type ReportTemplateConfig struct {
	ShowSummary bool `mapstructure:"show_summary"` // 是否显示汇总统计
	ShowGrowth  bool `mapstructure:"show_growth"`  // 是否显示用例增长趋势图
	ShowError   bool `mapstructure:"show_error"`   // 是否显示错误率趋势图
	ShowSlow    bool `mapstructure:"show_slow"`    // 是否显示慢接口排名
	ShowAlert   bool `mapstructure:"show_alert"`   // 是否显示连续失败告警
}

// AppConfig 存储全局配置实例
var AppConfig Config

// configMu 保护 AppConfig 的读写锁
var configMu sync.RWMutex

// OnChangeFunc 配置变更回调函数类型
type OnChangeFunc func(newCfg Config)

var (
	onChangeCallbacks []OnChangeFunc
	callbackMu        sync.Mutex
)

// GetConfig 安全地获取当前配置的副本
func GetConfig() Config {
	configMu.RLock()
	defer configMu.RUnlock()
	return AppConfig
}

// RegisterOnChange 注册配置变更回调，在配置热加载后调用
func RegisterOnChange(fn OnChangeFunc) {
	callbackMu.Lock()
	defer callbackMu.Unlock()
	onChangeCallbacks = append(onChangeCallbacks, fn)
}

// InitConfig 初始化配置
// 从配置文件读取配置并解析到 AppConfig 中
func InitConfig() {
	// 如果指定了配置文件路径，使用指定的文件
	if CfgFile != "" {
		viper.SetConfigFile(CfgFile)
		logger.Debug("使用指定配置文件", zap.String("路径", CfgFile))
	} else {
		// 默认从 ./config/config.yaml 读取配置
		viper.AddConfigPath("./config")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		logger.Debug("使用默认配置路径 ./config/config.yaml")
	}

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("读取配置文件失败: %v", err)
	}
	logger.Debug("配置文件读取成功")

	// 将配置解析到结构体（vars 字段会被 viper 转换为小写，后续会修复）
	if err := viper.Unmarshal(&AppConfig); err != nil {
		log.Fatalf("解析配置到结构体失败: %v", err)
	}
	logger.Debug("配置解析成功",
		zap.String("基础URL", AppConfig.Target.BaseURL),
		zap.Int("超时", AppConfig.Target.Timeout),
		zap.String("日志级别", AppConfig.Log.Level),
		zap.String("日志输出", AppConfig.Log.Output),
		zap.Int("轮数", AppConfig.Test.Rounds),
		zap.Int("间隔ms", AppConfig.Test.IntervalMs))

	// 设置清理器的默认配置
	applyCleanerDefaults(&AppConfig)
	logger.Debug("清理器默认值已应用",
		zap.Bool("启用", AppConfig.Cleaner.Enabled),
		zap.Int("保留天数", AppConfig.Cleaner.RetentionDays),
		zap.Int("间隔小时", AppConfig.Cleaner.IntervalHours))

	// 设置报告的默认配置
	applyReportingDefaults(&AppConfig)
	logger.Debug("报告默认值已应用",
		zap.String("发送时间", AppConfig.Reporting.SendTime),
		zap.Bool("日报", AppConfig.Reporting.DailyEnabled),
		zap.Bool("周报", AppConfig.Reporting.WeeklyEnabled),
		zap.Bool("月报", AppConfig.Reporting.MonthlyEnabled),
		zap.Bool("年报", AppConfig.Reporting.YearlyEnabled),
		zap.Int("连续失败阈值", AppConfig.Reporting.ConsecutiveFailN),
		zap.Int("慢接口上限", AppConfig.Reporting.TopSlowN))

	// 单独读取 vars 配置，保留原始键名（避免 viper 自动转换小写）
	AppConfig.Vars = loadRawVars()
	logger.Debug("自定义变量已加载", zap.Int("数量", len(AppConfig.Vars)))

	// 启动配置文件监听（热加载）
	go watchConfig()
	logger.Debug("配置文件热加载监听已启动")
}

// applyCleanerDefaults 设置 cleaner 配置的默认值
// 如果用户完全没有配置 cleaner，则启用默认配置
// 如果用户配置了 cleaner 的某些字段，则只为空字段设置默认值
func applyCleanerDefaults(cfg *Config) {
	// 检查配置文件中是否存在 cleaner 配置
	hasCleanerConfig := viper.IsSet("cleaner")

	// 如果用户完全没有配置 cleaner，启用默认配置（包括 enabled: true）
	if !hasCleanerConfig {
		cfg.Cleaner.Enabled = true
		cfg.Cleaner.RetentionDays = 30
		cfg.Cleaner.LogDir = "./logs"
		cfg.Cleaner.ReportDir = "./reports"
		cfg.Cleaner.DataDir = "./sql"
		cfg.Cleaner.IncludePatterns = []string{"*.log", "*.json", "*.csv", "*.txt"}
		cfg.Cleaner.IntervalHours = 24
		return
	}

	// 如果用户配置了 cleaner，但某些字段为空，则只为空字段设置默认值
	if cfg.Cleaner.RetentionDays <= 0 {
		cfg.Cleaner.RetentionDays = 30
	}
	if cfg.Cleaner.LogDir == "" {
		cfg.Cleaner.LogDir = "./logs"
	}
	if cfg.Cleaner.ReportDir == "" {
		cfg.Cleaner.ReportDir = "./reports"
	}
	if cfg.Cleaner.DataDir == "" {
		cfg.Cleaner.DataDir = "./sql"
	}
	if len(cfg.Cleaner.IncludePatterns) == 0 {
		cfg.Cleaner.IncludePatterns = []string{"*.log", "*.json", "*.csv", "*.txt"}
	}
	if cfg.Cleaner.IntervalHours <= 0 {
		cfg.Cleaner.IntervalHours = 24
	}
}

// applyReportingDefaults 设置 reporting 配置的默认值
func applyReportingDefaults(cfg *Config) {
	// 报告发送时间默认为早上5:00
	if cfg.Reporting.SendTime == "" {
		cfg.Reporting.SendTime = "05:00"
	}
	// 报告默认全部开启
	if !viper.IsSet("reporting.weekly_enabled") {
		cfg.Reporting.WeeklyEnabled = true
	}
	if !viper.IsSet("reporting.monthly_enabled") {
		cfg.Reporting.MonthlyEnabled = true
	}
	if !viper.IsSet("reporting.yearly_enabled") {
		cfg.Reporting.YearlyEnabled = true
	}
	if !viper.IsSet("reporting.daily_enabled") {
		cfg.Reporting.DailyEnabled = true
	}
	if !viper.IsSet("reporting.daily_summary") {
		cfg.Reporting.DailySummary = true
	}

	// 模板默认值：全部显示
	applyTemplateDefaults(&cfg.Reporting.DayTemplate)
	applyTemplateDefaults(&cfg.Reporting.WeekTemplate)
	applyTemplateDefaults(&cfg.Reporting.MonthTemplate)
	applyTemplateDefaults(&cfg.Reporting.YearTemplate)
}

// applyTemplateDefaults 设置模板配置的默认值（全部显示）
func applyTemplateDefaults(t *ReportTemplateConfig) {
	if !viper.IsSet("reporting.day_template") && !viper.IsSet("reporting.week_template") &&
		!viper.IsSet("reporting.month_template") && !viper.IsSet("reporting.year_template") {
		// 如果用户完全没有配置任何模板，则全部默认开启
		t.ShowSummary = true
		t.ShowGrowth = true
		t.ShowError = true
		t.ShowSlow = true
		t.ShowAlert = true
	}
}

// watchConfig 监听配置文件变化并热加载
func watchConfig() {
	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		logger.Info("检测到配置文件变化",
			zap.String("文件", e.Name),
			zap.String("操作", e.Op.String()))

		// 重新解析配置到临时结构体
		var newCfg Config
		if err := viper.Unmarshal(&newCfg); err != nil {
			logger.Error("解析新配置失败，保持旧配置", zap.Error(err))
			return
		}

		// 设置 cleaner 默认值
		applyCleanerDefaults(&newCfg)

		// 设置 reporting 默认值
		applyReportingDefaults(&newCfg)

		// 单独读取 vars 配置，保留原始键名
		newCfg.Vars = loadRawVars()

		// 原子更新全局配置
		configMu.Lock()
		AppConfig = newCfg
		configMu.Unlock()

		logger.Info("配置热加载成功")

		// 通知所有注册的回调
		callbackMu.Lock()
		callbacks := make([]OnChangeFunc, len(onChangeCallbacks))
		copy(callbacks, onChangeCallbacks)
		callbackMu.Unlock()

		for _, fn := range callbacks {
			fn(newCfg)
		}
	})
}

// loadRawVars 从配置文件读取原始 vars，保留键名大小写。
// viper 默认会把所有配置键转为小写，因此需要直接解析 YAML 文件来保留原始键名。
func loadRawVars() map[string]string {
	result := make(map[string]string)

	// 确定配置文件路径（不依赖 viper.ConfigFileUsed，避免某些场景返回空）
	configFile := CfgFile
	if configFile == "" {
		configFile = "./config/config.yaml"
	}

	// 直接从文件读取原始 YAML，保留 vars 键名大小写
	data, err := os.ReadFile(configFile)
	if err == nil {
		var raw map[string]any
		if err := yaml.Unmarshal(data, &raw); err == nil {
			if varsMap, ok := raw["vars"].(map[string]any); ok {
				for k, v := range varsMap {
					switch val := v.(type) {
					case string:
						result[k] = val
					default:
						result[k] = fmt.Sprintf("%v", val)
					}
				}
				logger.Debug("从 YAML 直接解析变量成功", zap.Int("数量", len(result)))
				return result
			}
		}
	}

	// 回退：使用 viper 读取（键名会被转小写）
	for k, v := range viper.GetStringMapString("vars") {
		result[k] = v
	}
	logger.Debug("回退到 viper 解析变量", zap.Int("数量", len(result)))
	return result
}