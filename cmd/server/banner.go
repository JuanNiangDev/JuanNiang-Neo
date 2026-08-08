package main

import (
	_ "embed"
	"strings"

	"github.com/fatih/color"
)

//go:embed banner.txt
var bannerText string

// printBanner 在启动时打印 ASCII banner（亮蓝色）。
// banner 文本由 go:embed 从 banner.txt 嵌入，避免在源码中手写大量反斜杠转义。
func printBanner() {
	color.New(color.FgHiBlue).Println(strings.TrimRight(bannerText, "\n"))
}
