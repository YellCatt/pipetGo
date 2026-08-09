package main

import (
	"pipetGo/cmd"
	"pipetGo/internal/app"
)

func main() {
	ctx, cancel := app.NewAppContext()
	defer cancel()

	cmd.ExecuteContext(ctx)
}
