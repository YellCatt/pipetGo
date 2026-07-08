// Package config 提供配置管理功能
// 使用 viper 读取 YAML 配置文件并解析到结构体中
package config

import (
	"log"
	"strings"

	"github.com/spf13/viper"
)

// CfgFile 存储命令行指定的配置文件路径
var CfgFile string

// Config 表示应用程序的完整配置
type Config struct {
	Target TargetConfig `mapstructure:"target"` // 目标 API 配置
	Log    LogConfig    `mapstructure:"log"`    // 日志配置
	Test   TestConfig   `mapstructure:"test"`   // 测试配置
	Email  EmailConfig  `mapstructure:"email"`  // 邮件配置
}

// TargetConfig 表示目标 API 的配置
type TargetConfig struct {
	BaseURL string `mapstructure:"base_url"` // API 基础地址
	Timeout int    `mapstructure:"timeout"`  // 请求超时时间（秒）
}

// LogConfig 表示日志系统的配置
type LogConfig struct {
	Level    string `mapstructure:"level"`    // 日志级别 (debug/info/warn/error)
	Encoding string `mapstructure:"encoding"` // 日志格式 (json/console)
	Output   string `mapstructure:"output"`   // 输出位置 (stdout 或文件路径)
}

// TestConfig 表示测试相关的配置
type TestConfig struct {
	ReportDir   string `mapstructure:"report_dir"`    // 测试报告输出目录
	TestCaseDir string `mapstructure:"test_case_dir"` // 默认测试用例目录
	DataDir     string `mapstructure:"data_dir"`      // 数据存储目录（用于SQLite数据库）
}

// EmailConfig 表示邮件发送相关的配置
type EmailConfig struct {
	From       string `mapstructure:"from"`        // 发件人邮箱
	To         string `mapstructure:"to"`          // 收件人邮箱
	AuthCode   string `mapstructure:"auth_code"`   // 邮箱授权码
	SMTPServer string `mapstructure:"smtp_server"` // SMTP 服务器地址
	SMTPPort   int    `mapstructure:"smtp_port"`   // SMTP 端口
}

// AppConfig 存储全局配置实例
var AppConfig Config

// InitConfig 初始化配置
// 从配置文件读取配置并解析到 AppConfig 中
func InitConfig() {
	// 如果指定了配置文件路径，使用指定的文件
	if CfgFile != "" {
		viper.SetConfigFile(CfgFile)
	} else {
		// 默认从 ./config/config.yaml 读取配置
		viper.AddConfigPath("./config")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	// 启用环境变量替换
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	// 将配置解析到结构体
	if err := viper.Unmarshal(&AppConfig); err != nil {
		log.Fatalf("Unable to decode config into struct: %v", err)
	}
}