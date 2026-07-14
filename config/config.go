// Package config 提供配置管理功能
// 使用 viper 读取 YAML 配置文件并解析到结构体中
package config

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// CfgFile 存储命令行指定的配置文件路径
var CfgFile string

// Config 表示应用程序的完整配置
type Config struct {
	Target TargetConfig      `mapstructure:"target"` // 目标 API 配置
	Log    LogConfig         `mapstructure:"log"`    // 日志配置
	Test   TestConfig        `mapstructure:"test"`   // 测试配置
	HTTP   HTTPConfig        `mapstructure:"http"`   // HTTP 客户端配置
	Email  EmailConfig       `mapstructure:"email"`  // 邮件配置
	Vars   map[string]string `mapstructure:"vars"`   // 用户自定义变量（用于替换测试用例中的 {{var}}）
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
	ReportDir     string   `mapstructure:"report_dir"`     // 测试报告输出目录
	TestCaseDir   string   `mapstructure:"test_case_dir"`  // 默认测试用例目录
	DataDir       string   `mapstructure:"data_dir"`       // 数据存储目录（用于 CSV 文件）

	SevereStatus  []int    `mapstructure:"severe_status"`  // 严重错误状态码列表，这些状态码的测试用例失败时优先于其他失败用例
	GlobalPre     []string `mapstructure:"global_pre"`     // 全局前置条件测试用例ID列表（所有测试执行前运行）
	GlobalPost    []string `mapstructure:"global_post"`    // 全局后置条件测试用例ID列表（所有测试执行后运行）
	DeviceName    string   `mapstructure:"device_name"`    // 测试设备名称（未配置时自动使用主机名）
}

// EmailConfig 表示邮件发送相关的配置
type EmailConfig struct {
	Enabled    bool   `mapstructure:"enabled"`    // 是否启用邮件发送
	From       string `mapstructure:"from"`       // 发件人邮箱
	To         string `mapstructure:"to"`         // 收件人邮箱
	AuthCode   string `mapstructure:"auth_code"`  // 邮箱授权码
	SMTPServer string `mapstructure:"smtp_server"` // SMTP 服务器地址
	SMTPPort   int    `mapstructure:"smtp_port"`  // SMTP 端口
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

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	// 将配置解析到结构体（vars 字段会被 viper 转换为小写，后续会修复）
	if err := viper.Unmarshal(&AppConfig); err != nil {
		log.Fatalf("Unable to decode config into struct: %v", err)
	}

	// 单独读取 vars 配置，保留原始键名（避免 viper 自动转换小写）
	AppConfig.Vars = loadRawVars()
}

// loadRawVars 从配置文件读取原始 vars，保留键名大小写。
// viper 默认会把所有配置键转为小写，因此需要直接解析 YAML 文件来保留原始键名。
func loadRawVars() map[string]string {
	result := make(map[string]string)

	// 优先从配置文件读取原始 YAML，保留键名大小写
	configFile := viper.ConfigFileUsed()
	if configFile == "" && CfgFile != "" {
		configFile = CfgFile
	}

	if configFile != "" {
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
					return result
				}
			}
		}
	}

	// 回退：使用 viper 读取（键名会被转小写）
	for k, v := range viper.GetStringMapString("vars") {
		result[k] = v
	}
	return result
}