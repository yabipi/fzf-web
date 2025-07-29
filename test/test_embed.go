package main

import (
	"embed"
	"fmt"
	"log"
)

//go:embed cmd/assets
var assets embed.FS

func main() {
	// 列出嵌入的文件
	entries, err := assets.ReadDir("cmd/assets")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("嵌入的文件:")
	for _, entry := range entries {
		fmt.Printf("- %s\n", entry.Name())
	}

	// 读取HTML文件
	content, err := assets.ReadFile("cmd/assets/index.html")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\nHTML文件大小: %d 字节\n", len(content))
	fmt.Printf("HTML文件前100个字符: %s\n", string(content[:100]))
}
