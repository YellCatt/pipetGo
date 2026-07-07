package config

import (
	"log"
	"strings"

	"github.com/spf13/viper"
)

var CfgFile string

type Config struct {
	Target TargetConfig `mapstructure:"target"`
	Log    LogConfig    `mapstructure:"log"`
	Test   TestConfig   `mapstructure:"test"`
}

type TargetConfig struct {
	BaseURL string `mapstructure:"base_url"`
	Timeout int    `mapstructure:"timeout"`
}

type LogConfig struct {
	Level    string `mapstructure:"level"`
	Encoding string `mapstructure:"encoding"`
	Output   string `mapstructure:"output"`
}

type TestConfig struct {
	ReportDir   string `mapstructure:"report_dir"`
	TestCaseDir string `mapstructure:"test_case_dir"`
}

var AppConfig Config

func InitConfig() {
	if CfgFile != "" {
		viper.SetConfigFile(CfgFile)
	} else {
		viper.AddConfigPath("./config")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	if err := viper.Unmarshal(&AppConfig); err != nil {
		log.Fatalf("Unable to decode config into struct: %v", err)
	}
}