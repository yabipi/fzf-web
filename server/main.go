package main

import (
	"log"
	"net/http"
)

func _main() {
	// 设置静态文件服务
	fs := http.FileServer(http.Dir("../static"))
	http.Handle("/", fs)

	// 设置WASM文件服务
	http.HandleFunc("/security.wasm", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/wasm")
		http.ServeFile(w, r, "../js/security.wasm")
	})

	// 设置exec.js服务
	http.HandleFunc("/exec.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, "../js/exec.js")
	})

	// 启动服务器
	log.Println("Server starting on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
