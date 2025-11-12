package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	fzf "github.com/junegunn/fzf/src"
)

//go:embed assets
var assets embed.FS

type SearchResult struct {
	Path     string `json:"path"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

type SearchRequest struct {
	Query   string `json:"query"`
	BaseDir string `json:"baseDir"`
	UseAPI  bool   `json:"useAPI"` // 是否使用 fzf API
}

type SearchResponse struct {
	Results []SearchResult         `json:"results"`
	Error   string                 `json:"error,omitempty"`
	Debug   map[string]interface{} `json:"debug,omitempty"` // 调试信息
}

var (
	baseDir   string // 搜索目录
	port      string // 服务器端口
	templates *template.Template
)

func init() {
	// 读取嵌入的HTML模板
	htmlContent, err := assets.ReadFile("assets/index.html")
	if err != nil {
		log.Fatalf("无法读取HTML模板: %v", err)
	}

	// 解析HTML模板
	templates = template.Must(template.New("index").Parse(string(htmlContent)))
}

func main() {
	// 获取当前目录作为默认搜索目录
	currentDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("无法获取当前目录: %v", err)
	}

	// 解析命令行参数
	flag.StringVar(&baseDir, "d", currentDir, "指定搜索目录 (简写)")
	flag.StringVar(&baseDir, "dir", currentDir, "指定搜索目录")
	flag.StringVar(&port, "p", "8080", "指定服务器端口 (简写)")
	flag.StringVar(&port, "port", "8080", "指定服务器端口")
	flag.Parse()

	// 检查目录是否存在
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		log.Fatalf("指定的搜索目录不存在: %s", baseDir)
	}

	// 设置静态文件路由
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/api/search", handleSearch)
	http.HandleFunc("/api/download", handleDownload)

	// 设置静态文件服务
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// 确保端口格式正确（添加冒号前缀）
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	fmt.Printf("启动服务器在 http://localhost%s\n", port)
	fmt.Printf("搜索目录: %s\n", baseDir)
	fmt.Printf("使用 -d 或 --dir 参数可以指定其他搜索目录\n")
	fmt.Printf("使用 -p 或 --port 参数可以指定服务器端口\n")
	fmt.Printf("示例: go run fzf-web.go -d /path/to/search -p 3000\n")
	log.Fatal(http.ListenAndServe(port, nil))
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"BaseDir": baseDir,
	}
	templates.ExecuteTemplate(w, "index", data)
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 使用提供的查询和目录
	query := req.Query
	searchDir := req.BaseDir
	if searchDir == "" {
		searchDir = baseDir
	}

	// 检查目录是否存在
	if _, err := os.Stat(searchDir); os.IsNotExist(err) {
		json.NewEncoder(w).Encode(SearchResponse{
			Error: "目录不存在: " + searchDir,
		})
		return
	}

	// 执行fzf搜索
	var results []SearchResult
	var err error
	var debugInfo = make(map[string]interface{})

	// 首先尝试使用 fzf API 搜索
	results, err = executeFzfSearchAPI(query, searchDir)

	// 如果 fzf 搜索失败或没有结果，尝试简单搜索
	if err != nil || len(results) == 0 {
		fmt.Printf("fzf 搜索失败或无结果，尝试简单搜索: %v\n", err)
		debugInfo["fzf_error"] = err.Error()
		simpleResults, simpleErr := executeSimpleSearch(query, searchDir)
		if simpleErr == nil && len(simpleResults) > 0 {
			results = simpleResults
			err = nil
			debugInfo["fallback_search"] = "使用简单搜索成功"
			fmt.Printf("简单搜索成功，找到 %d 个结果\n", len(results))
		} else {
			debugInfo["simple_search_error"] = simpleErr.Error()
		}
	} else {
		debugInfo["search_method"] = "fzf API"
	}

	debugInfo["query"] = query
	debugInfo["search_dir"] = searchDir
	debugInfo["results_count"] = len(results)

	if err != nil {
		json.NewEncoder(w).Encode(SearchResponse{
			Error: "搜索失败: " + err.Error(),
			Debug: debugInfo,
		})
		return
	}

	json.NewEncoder(w).Encode(SearchResponse{
		Results: results,
		Debug:   debugInfo,
	})
}

//func executeFzfSearch(query, searchDir string) ([]SearchResult, error) {
//	// 获取所有文件列表
//	files, err := getAllFiles(searchDir)
//	if err != nil {
//		return nil, err
//	}
//
//	// 构建fzf命令，使用filter模式（非交互）
//	cmd := exec.Command("fzf", "--filter", query, "--no-mouse", "--no-color", "--print-query")
//	cmd.Dir = searchDir
//
//	// 将文件列表写入fzf的标准输入
//	stdin, err := cmd.StdinPipe()
//	if err != nil {
//		return nil, err
//	}
//
//	// 启动命令
//	if err := cmd.Start(); err != nil {
//		return nil, fmt.Errorf("fzf命令启动失败: %v", err)
//	}
//
//	// 写入文件列表到fzf并关闭stdin
//	go func() {
//		defer stdin.Close()
//		for _, file := range files {
//			fmt.Fprintln(stdin, file)
//		}
//	}()
//
//	// 等待命令完成并读取输出
//	output, err := cmd.Output()
//	if err != nil {
//		// fzf在没有匹配时返回非零退出码，这是正常的
//		if strings.Contains(err.Error(), "exit status 1") {
//			return []SearchResult{}, nil
//		}
//		return nil, fmt.Errorf("fzf执行失败: %v, output: %s", err, string(output))
//	}
//
//	// 解析输出
//	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
//	var results []SearchResult
//
//	for _, line := range lines {
//		line = strings.TrimSpace(line)
//		if line == "" {
//			continue // 跳过空行
//		}
//
//		// 跳过查询行（--print-query 会输出查询）
//		if line == query {
//			continue
//		}
//
//		fullPath := filepath.Join(searchDir, line)
//		info, err := os.Stat(fullPath)
//		if err != nil {
//			continue
//		}
//
//		results = append(results, SearchResult{
//			Path:     line,
//			Filename: filepath.Base(line),
//			Size:     info.Size(),
//		})
//	}
//
//	return results, nil
//}

// executeFzfSearchAPI 使用 fzf 的 Go API 进行搜索
func executeFzfSearchAPI(query, searchDir string) ([]SearchResult, error) {
	// 获取所有文件列表
	files, err := getAllFiles(searchDir)
	if err != nil {
		return nil, err
	}

	// 限制文件数量，避免处理过多文件

	// if len(files) > 15000 {
	// 	files = files[:15000]
	// 	fmt.Printf("文件数量过多，限制为 %d 个文件进行搜索\n", len(files))
	// }

	//if len(files) > 10000 {
	//	files = files[:10000]
	//}

	// 添加调试信息
	fmt.Printf("开始搜索，查询: %s, 文件数量: %d\n", query, len(files))

	// 创建输入通道
	inputChan := make(chan string, len(files))

	// 创建输出通道
	outputChan := make(chan string, 100)

	// 创建结果收集通道
	resultsChan := make(chan []SearchResult, 1)

	// 在 goroutine 中收集输出
	go func() {
		var results []SearchResult
		for s := range outputChan {
			line := strings.TrimSpace(s)
			if line == "" || line == query {
				continue // 跳过空行和查询行
			}

			fullPath := filepath.Join(searchDir, line)
			info, err := os.Stat(fullPath)
			if err != nil {
				continue
			}

			results = append(results, SearchResult{
				Path:     line,
				Filename: filepath.Base(line),
				Size:     info.Size(),
			})
		}
		resultsChan <- results
	}()

	// 构建 fzf 选项 - 改进搜索选项
	options, err := fzf.ParseOptions(
		false, // 不加载默认选项，避免冲突
		[]string{
			"--filter", query,
			"--no-mouse",
			"--no-color",
			"--print-query",
			"--extended", // 启用扩展搜索模式
			"--no-sort",  // 不排序，保持原始顺序
			"--tac",      // 反转输入顺序，新文件在前
		},
	)
	if err != nil {
		return nil, fmt.Errorf("fzf 选项解析失败: %v", err)
	}

	// 设置输入和输出通道
	options.Input = inputChan
	options.Output = outputChan

	// 启动 fzf
	go func() {
		defer close(outputChan)
		code, err := fzf.Run(options)
		if err != nil {
			fmt.Printf("fzf 运行错误: %v\n", err)
		}
		if code != fzf.ExitOk && code != fzf.ExitNoMatch {
			fmt.Printf("fzf 异常退出，退出码: %d\n", code)
		}
	}()

	// 发送文件列表到输入通道
	go func() {
		defer close(inputChan)
		for _, file := range files {
			inputChan <- file
		}
	}()

	// 等待结果收集完成
	results := <-resultsChan

	// 添加调试信息
	fmt.Printf("搜索完成，找到 %d 个结果\n", len(results))

	return results, nil
}

// executeSimpleSearch 使用简单的字符串匹配作为备用搜索方法
func executeSimpleSearch(query, searchDir string) ([]SearchResult, error) {
	// 获取所有文件列表
	files, err := getAllFiles(searchDir)
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(query)
	var results []SearchResult

	for _, file := range files {
		filename := strings.ToLower(filepath.Base(file))
		path := strings.ToLower(file)

		// 检查文件名或路径是否包含查询
		if strings.Contains(filename, query) || strings.Contains(path, query) {
			fullPath := filepath.Join(searchDir, file)
			info, err := os.Stat(fullPath)
			if err != nil {
				continue
			}

			results = append(results, SearchResult{
				Path:     file,
				Filename: filepath.Base(file),
				Size:     info.Size(),
			})
		}
	}

	fmt.Printf("简单搜索完成，找到 %d 个结果\n", len(results))
	return results, nil
}

func getAllFiles(dir string) ([]string, error) {
	var files []string
	count := 0
	maxFiles := 100000 // 增加文件数量限制

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 限制文件数量
		if count >= maxFiles {
			return filepath.SkipAll
		}

		// 跳过隐藏文件和目录（但保留一些重要的隐藏文件）
		if strings.HasPrefix(filepath.Base(path), ".") {
			if info.IsDir() {
				// 对于隐藏目录，检查是否包含重要文件
				baseName := filepath.Base(path)
				if baseName == ".git" || baseName == ".vscode" || baseName == ".idea" {
					// 保留这些目录，但跳过其中的大部分文件
					return nil
				}
				return filepath.SkipDir
			}
			// 对于隐藏文件，保留一些重要的配置文件
			baseName := filepath.Base(path)
			if baseName == ".gitignore" || baseName == ".env" || baseName == ".env.local" ||
				strings.HasSuffix(baseName, ".config") || strings.HasSuffix(baseName, ".conf") {
				// 保留这些重要文件
			} else {
				return nil // 跳过其他隐藏文件
			}
		}

		// 跳过一些常见的系统目录
		if info.IsDir() {
			baseName := filepath.Base(path)
			if baseName == "node_modules" || baseName == "__pycache__" ||
				baseName == "vendor" || baseName == "target" || baseName == "build" ||
				baseName == "dist" || baseName == ".next" || baseName == ".nuxt" {
				return filepath.SkipDir
			}
			return nil
		}

		// 只包含文件，不包含目录
		if !info.IsDir() {
			// 返回相对路径
			relPath, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			files = append(files, relPath)
			count++
		}

		return nil
	})

	// 添加调试信息
	fmt.Printf("扫描目录 %s，找到 %d 个文件\n", dir, len(files))
	if len(files) >= maxFiles {
		fmt.Printf("警告：文件数量达到限制 %d，可能遗漏了一些文件\n", maxFiles)
	}

	return files, err
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("file")
	searchDir := r.URL.Query().Get("dir") // 获取搜索目录参数

	if filePath == "" {
		http.Error(w, "Missing file parameter", http.StatusBadRequest)
		return
	}

	// 如果没有指定搜索目录，使用默认的 baseDir
	if searchDir == "" {
		searchDir = baseDir
	}

	// 构建完整路径
	fullPath := filepath.Join(searchDir, filePath)

	// 安全检查：确保文件在指定目录内
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	absSearchDir, err := filepath.Abs(searchDir)
	if err != nil {
		http.Error(w, "Invalid search directory", http.StatusInternalServerError)
		return
	}

	if !strings.HasPrefix(absPath, absSearchDir) {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	// 检查文件是否存在
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// 设置下载头
	filename := filepath.Base(filePath)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Type", "application/octet-stream")

	// 提供文件下载
	http.ServeFile(w, r, fullPath)
}
