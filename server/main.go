package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
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

type baseResponse struct {
	Code    uint32      `json:"code"`
	Message string      `json:"msg"`
	Data    interface{} `json:"data"`
}

func main() {
	// 加载上传文件页面
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		tpl, err := template.ParseFiles("./server/upload.html")
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
		if req.Method != http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			resBytes, _ := json.Marshal(baseResponse{
				Code:    404,
				Message: "Request Path Not Found!",
			})
			w.Write(resBytes)
			return
		}
		resBytes, _ := json.Marshal(baseResponse{
			Code:    200,
			Message: "Success!",
			Data:    "World!",
		})
		count, err := w.Write(resBytes)
		fmt.Printf("on Get req come, res: %d, err: %v\n", count, err)
	})

	http.HandleFunc("/json", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if req.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			resBytes, _ := json.Marshal(baseResponse{
				Code:    404,
				Message: "Request Path Not Found!",
			})
			w.Write(resBytes)
			return
		}
		bodyBytes, _ := ioutil.ReadAll(req.Body)
		resBytes, _ := json.Marshal(baseResponse{
			Code:    200,
			Message: "Success!",
			Data:    string(bodyBytes),
		})
		count, err := w.Write(resBytes)
		fmt.Printf("on Post req come, res: %d, err: %v\n", count, err)
	})

	http.HandleFunc("/head", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodHead {
			w.WriteHeader(http.StatusNotFound)
			resBytes, _ := json.Marshal(baseResponse{
				Code:    404,
				Message: "Request Path Not Found!",
			})
			w.Write(resBytes)
			return
		}
		w.WriteHeader(200)
		fmt.Printf("on Head req come")
	})

	go func() {
		log.Println("Server started on localhost:888")
		_ = http.ListenAndServe(":888", nil)
	}()

	go func() {
		log.Println("Server started on localhost:8443")
		_ = http.ListenAndServeTLS("192.168.80.239:8443", "./server/cert.pem", "./server/key.pem", nil)
	}()
	select {}
}
