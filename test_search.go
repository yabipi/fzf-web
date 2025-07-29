package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

type SearchRequest struct {
	Query   string `json:"query"`
	BaseDir string `json:"baseDir"`
}

type SearchResponse struct {
	Results []SearchResult         `json:"results"`
	Error   string                 `json:"error,omitempty"`
	Debug   map[string]interface{} `json:"debug,omitempty"`
}

type SearchResult struct {
	Path     string `json:"path"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: go run test_search.go <搜索关键词>")
		return
	}

	query := os.Args[1]
	baseDir := "."
	if len(os.Args) > 2 {
		baseDir = os.Args[2]
	}

	// 构建请求
	req := SearchRequest{
		Query:   query,
		BaseDir: baseDir,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		log.Fatal("JSON编码失败:", err)
	}

	// 发送请求
	resp, err := http.Post("http://localhost:8080/api/search", "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		log.Fatal("请求失败:", err)
	}
	defer resp.Body.Close()

	// 解析响应
	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		log.Fatal("响应解析失败:", err)
	}

	// 显示结果
	if searchResp.Error != "" {
		fmt.Printf("搜索错误: %s\n", searchResp.Error)
	}

	if searchResp.Debug != nil {
		fmt.Println("\n=== 调试信息 ===")
		for key, value := range searchResp.Debug {
			fmt.Printf("%s: %v\n", key, value)
		}
	}

	fmt.Printf("\n=== 搜索结果 (%d 个) ===\n", len(searchResp.Results))
	for i, result := range searchResp.Results {
		fmt.Printf("%d. %s (%s) - %s\n", i+1, result.Filename, result.Path, formatFileSize(result.Size))
	}
}

func formatFileSize(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}
	const k = 1024
	sizes := []string{"B", "KB", "MB", "GB"}
	i := 0
	for bytes >= k && i < len(sizes)-1 {
		bytes /= k
		i++
	}
	return fmt.Sprintf("%.2f %s", float64(bytes), sizes[i])
}
