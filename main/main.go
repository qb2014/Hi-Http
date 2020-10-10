package main

import (
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
)

const maxBytes = 2 * 1024 * 1024 // 2 MB

// 处理文件上传
func uploadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		http.MaxBytesReader(w, req.Body, maxBytes)
		if err := req.ParseMultipartForm(maxBytes); err != nil {
			log.Printf("req.ParseMultipartForm: %v", err)
			return
		}
		file, _, err := req.FormFile("file")
		if err != nil {
			log.Printf("req.FormFile: %v", err)
			return
		}
		defer func() { _ = file.Close() }()
		f, _ := os.Create("file.txt")
		defer func() { _ = f.Close() }()
		_, _ = io.Copy(f, file)
	}
}

func main() {
	// 加载上传文件页面
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		tpl, err := template.ParseFiles("./upload.html")
		if err != nil {
			log.Printf("template.New: %v", err)
			return
		}

		if err := tpl.Execute(w, nil); err != nil {
			log.Printf("tpl.Execute: %v", err)
			return
		}
	})
	http.HandleFunc("/upload", uploadHandler())

	fs := http.FileServer(http.Dir("./tmp"))
	http.Handle("/files", http.StripPrefix("/files", fs))

	http.HandleFunc("/hello", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte("World!"))
	})

	log.Println("Server started on localhost:8080")

	log.Fatal(http.ListenAndServe(":8080", nil))
}
