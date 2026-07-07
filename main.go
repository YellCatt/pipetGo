// Package main 是 pipet 项目的主入口
// pipet 是一个功能强大的企业级 API 测试工具
package main

import (
	// cmd 包包含命令行接口的实现
	"pipet/cmd"
)

// main 函数是程序的入口点
// 调用 cmd.Execute() 启动命令行应用
func main() {
	cmd.Execute()
}