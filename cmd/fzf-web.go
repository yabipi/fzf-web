package main

import (
	"archive/zip"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

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
	Query  string `json:"query"`
	UseAPI bool   `json:"useAPI"` // 是否使用 fzf API
}

type SearchResponse struct {
	Results []SearchResult `json:"results"`
	Error   string         `json:"error,omitempty"`
}

type BatchDownloadRequest struct {
	Files []string `json:"files"`
	Query string   `json:"query"`
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
	http.HandleFunc("/api/download-batch", handleBatchDownload)

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
	fmt.Printf("示例: go run ./cmd -d /path/to/search -p 3000\n")
	log.Fatal(http.ListenAndServe(port, nil))
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	// 不再需要传递BaseDir到模板
	templates.ExecuteTemplate(w, "index", nil)
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

	// 只使用命令行指定的目录，忽略用户请求中的目录
	query := req.Query
	searchDir := baseDir // 直接使用命令行指定的目录

	// 检查目录是否存在
	if _, err := os.Stat(searchDir); os.IsNotExist(err) {
		json.NewEncoder(w).Encode(SearchResponse{
			Error: "搜索目录不存在: " + searchDir,
		})
		return
	}

	// 执行fzf搜索
	var results []SearchResult
	var err error

	filterQuery := buildFzfFilterQuery(query)
	if filterQuery == "" {
		json.NewEncoder(w).Encode(SearchResponse{Results: []SearchResult{}})
		return
	}

	// 首先尝试使用 fzf API 搜索
	results, err = executeFzfSearchAPI(query, filterQuery, searchDir)

	// 如果 fzf 搜索失败或没有结果，尝试简单搜索
	if err != nil || len(results) == 0 {
		fmt.Printf("fzf 搜索失败或无结果，尝试简单搜索: %v\n", err)
		simpleResults, simpleErr := executeSimpleSearch(query, searchDir)
		if simpleErr == nil && len(simpleResults) > 0 {
			results = simpleResults
			err = nil
			fmt.Printf("简单搜索成功，找到 %d 个结果\n", len(results))
		}
	}

	if err != nil {
		json.NewEncoder(w).Encode(SearchResponse{
			Error: "搜索失败: " + err.Error(),
		})
		return
	}

	if results == nil {
		results = []SearchResult{}
	}

	json.NewEncoder(w).Encode(SearchResponse{
		Results: results,
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
func executeFzfSearchAPI(originalQuery, filterQuery, searchDir string) ([]SearchResult, error) {
	// 获取所有文件列表
	files, err := getAllFiles(searchDir)
	if err != nil {
		return nil, err
	}

	fmt.Printf("开始搜索，查询: %s, fzf 过滤: %s, 文件数量: %d\n", originalQuery, filterQuery, len(files))

	// 创建输入通道
	inputChan := make(chan string, len(files))

	// 输出通道缓冲区与文件数一致，避免大量匹配结果时阻塞
	outputBuf := len(files)
	if outputBuf < 100 {
		outputBuf = 100
	}
	outputChan := make(chan string, outputBuf)

	// 创建结果收集通道
	resultsChan := make(chan []SearchResult, 1)

	skipQueryLines := map[string]struct{}{
		originalQuery: {},
		filterQuery:   {},
	}

	// 在 goroutine 中收集输出
	go func() {
		var results []SearchResult
		for s := range outputChan {
			line := strings.TrimSpace(s)
			if line == "" {
				continue
			}
			if _, skip := skipQueryLines[line]; skip {
				continue // 跳过 --print-query 输出的查询行
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
			"--filter", filterQuery,
			"--no-mouse",
			"--no-color",
			"--print-query",
			"--extended", // 空格分隔多词为 AND
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
	if results == nil {
		results = []SearchResult{}
	}

	// 添加调试信息
	fmt.Printf("搜索完成，找到 %d 个结果\n", len(results))

	return results, nil
}

// executeSimpleSearch 使用简单的字符串匹配作为备用搜索方法
func executeSimpleSearch(query, searchDir string) ([]SearchResult, error) {
	terms := splitSearchTerms(query)
	if len(terms) == 0 {
		return []SearchResult{}, nil
	}

	// 获取所有文件列表
	files, err := getAllFiles(searchDir)
	if err != nil {
		return nil, err
	}

	var results []SearchResult

	for _, file := range files {
		filename := filepath.Base(file)
		if !matchesAllTerms(filename, terms) && !matchesAllTerms(file, terms) {
			continue
		}

		fullPath := filepath.Join(searchDir, file)
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}

		results = append(results, SearchResult{
			Path:     file,
			Filename: filename,
			Size:     info.Size(),
		})
	}

	fmt.Printf("简单搜索完成，找到 %d 个结果\n", len(results))
	if results == nil {
		results = []SearchResult{}
	}
	return results, nil
}

func getAllFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
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
		}

		return nil
	})

	fmt.Printf("扫描目录 %s，找到 %d 个文件\n", dir, len(files))

	return files, err
}

func resolveFileInSearchDir(filePath, searchDir string) (string, error) {
	if filePath == "" {
		return "", fmt.Errorf("empty file path")
	}

	fullPath := filepath.Join(searchDir, filePath)

	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("invalid file path")
	}

	absSearchDir, err := filepath.Abs(searchDir)
	if err != nil {
		return "", fmt.Errorf("invalid search directory")
	}

	absPath = filepath.Clean(absPath)
	absSearchDir = filepath.Clean(absSearchDir)
	if absPath != absSearchDir && !strings.HasPrefix(absPath, absSearchDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("access denied")
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return "", fmt.Errorf("file not found")
	}
	if info.IsDir() {
		return "", fmt.Errorf("not a file")
	}

	return fullPath, nil
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("file")

	fullPath, err := resolveFileInSearchDir(filePath, baseDir)
	if err != nil {
		switch err.Error() {
		case "empty file path", "invalid file path":
			http.Error(w, "Invalid file path", http.StatusBadRequest)
		case "access denied":
			http.Error(w, "Access denied", http.StatusForbidden)
		case "file not found", "not a file":
			http.Error(w, "File not found", http.StatusNotFound)
		default:
			http.Error(w, "Invalid search directory", http.StatusInternalServerError)
		}
		return
	}

	filename := filepath.Base(filePath)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, fullPath)
}

func zipFilenameFromQuery(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return "search.zip"
	}

	var b strings.Builder
	for _, r := range query {
		if unicode.IsControl(r) || r == '/' || r == '\\' || r == ':' ||
			r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			continue
		}
		b.WriteRune(r)
	}

	name := strings.TrimSpace(b.String())
	if name == "" {
		return "search.zip"
	}
	if len(name) > 100 {
		name = name[:100]
	}
	if !strings.HasSuffix(strings.ToLower(name), ".zip") {
		name += ".zip"
	}
	return name
}

func attachmentContentDisposition(filename string) string {
	ascii := filename
	for _, r := range filename {
		if r >= 128 || r == '"' || r == '\\' {
			ascii = "search.zip"
			break
		}
	}
	encoded := strings.ReplaceAll(url.QueryEscape(filename), "+", "%20")
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, encoded)
}

func handleBatchDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BatchDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Files) == 0 {
		http.Error(w, "No files selected", http.StatusBadRequest)
		return
	}

	var validFiles []string
	for _, filePath := range req.Files {
		if _, err := resolveFileInSearchDir(filePath, baseDir); err == nil {
			validFiles = append(validFiles, filePath)
		}
	}
	if len(validFiles) == 0 {
		http.Error(w, "No valid files to download", http.StatusBadRequest)
		return
	}

	zipName := zipFilenameFromQuery(req.Query)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", attachmentContentDisposition(zipName))

	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	for _, filePath := range validFiles {
		fullPath, err := resolveFileInSearchDir(filePath, baseDir)
		if err != nil {
			continue
		}

		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			continue
		}
		header.Name = filepath.Base(filePath)
		header.Method = zip.Deflate

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			continue
		}

		file, err := os.Open(fullPath)
		if err != nil {
			continue
		}

		_, copyErr := io.Copy(writer, file)
		file.Close()
		if copyErr != nil {
			continue
		}
	}
}
