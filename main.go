package main

import (
	"pipetGo/cmd"
	"pipetGo/config"
	"pipetGo/internal/app"
	"pipetGo/internal/logger"
)

func main() {
	app.InitDirectories()
	config.InitConfig()

	logger.InitLogger(logger.LogConfig{
		Level:    config.AppConfig.Log.Level,
		Encoding: config.AppConfig.Log.Encoding,
		Output:   config.AppConfig.Log.Output,
	})

	ctx, cancel := app.NewAppContext()
	defer cancel()

	cmd.ExecuteContext(ctx)
}
