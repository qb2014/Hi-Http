package SimpleHttpClient

import (
	"bytes"
	"context"
	"testing"
	"time"
)

/// -------------------------------- HTTP TESTING --------------------------------

// 测试Get请求
func TestHttpClient_Get(t *testing.T) {
	ctx := context.WithValue(context.Background(), "DEV", true)
	client, err := NewHttpClient(ctx, "http://localhost:888", Options{
		Retry: 0,
		TLS:   nil,
	})
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}
	defer client.End()
	result, err := client.Get("/hello")
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}
	t.Logf("success! result -> %s\n", result)
}

// 测试主动取消请求
func TestHttpClient_Get_With_Cancel(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	client, err := NewHttpClient(ctx, "http://localhost:888", Options{
		Retry: 1,
		TLS:   nil,
	})
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}
	defer client.End()
	go func() { time.Sleep(50 * time.Microsecond); cancelFunc() }()
	result, err := client.Get("/hello")
	if err != nil && err == ErrRequestCanceled {
		return
	}
	t.Logf("fail!(result -> %s), err: %v\n", result, err)
	t.Fail()
}

// 测试连接错误的服务器地址
func TestHttpClient_Get_With_Wrong_Conn(t *testing.T) {
	ctx := context.WithValue(context.Background(), "DEV", true)
	client, err := NewHttpClient(ctx, "http://localhost:8888", Options{
		Retry: 2,
		TLS:   nil,
	})
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}
	defer client.End()
	result, err := client.Get("/hello")
	if err != nil && err == ErrConnectingTimeout {
		return
	}
	t.Logf("fail!(result -> %s), err: %v\n", result, err)
	t.Fail()
}

// 测试连接错误的服务器地址
func TestHttpClient_Get_With_Timeout(t *testing.T) {
	ctx := context.WithValue(context.Background(), "DEV", true)
	client, err := NewHttpClient(ctx, "http://localhost:888", Options{
		Retry: 2,
		TLS:   nil,
	})
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}
	defer client.End()
	timeout := 100 * time.Millisecond
	client.SetTimeout(timeout)
	client.SetReadTimeout(timeout)
	client.SetWriteTimeout(timeout)
	result, err := client.Get("/hello")
	if err != nil && err == ErrRequestTimeout {
		t.Log(err)
		return
	}
	t.Logf("fail!(result -> %s), err: %v\n", result, err)
	t.Fail()
}

// 测试同一个客户端进行多次请求
func TestHttpClient_Get_With_Multi_Req(t *testing.T) {
	ctx := context.WithValue(context.Background(), "DEV", true)
	client, err := NewHttpClient(ctx, "http://localhost:888", Options{
		Retry: 0,
		TLS:   nil,
	})
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}
	defer client.End()
	type response struct {
		id  int
		res string
		err error
	}
	resChan := make(chan response, 3)
	go func() {
		result, err := client.Get("/")
		resChan <- response{id: 1, res: result, err: err}
	}()
	go func() {
		result, err := client.Get("/hello")
		resChan <- response{id: 2, res: result, err: err}
	}()
	go func() {
		result, err := client.Get("/hello")
		resChan <- response{id: 3, res: result, err: err}
	}()
	var taskCounter int
	for res := range resChan {
		if res.err != nil {
			t.Log("reqID:", res.id, ",res err:", res.err)
			t.Fail()
			return
		}
		t.Log("reqID:", res.id, ",on response:", res.res)
		taskCounter++
		if taskCounter > 2 {
			close(resChan)
			break
		}
	}
}

// 测试Post请求
func TestHttpClient_Post(t *testing.T) {
	ctx := context.WithValue(context.Background(), "DEV", true)
	client, err := NewHttpClient(ctx, "http://localhost:888", Options{
		Retry: 0,
		TLS:   nil,
	})
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}
	client.SetHeader("Content-Type", JSON)
	defer client.End()
	json := `{
	"name": "李鸿辉",
	"title": "Golang Developer",
	"phone": "18476577880",
	"email": "lpeng0_0h@163.com",
}`
	result, err := client.Post("/json", bytes.NewBufferString(json))
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}
	t.Logf("success! result -> %s\n", result)
}

// 测试Head请求
func TestHttpClient_Head(t *testing.T) {
	ctx := context.WithValue(context.Background(), "DEV", true)
	client, err := NewHttpClient(ctx, "http://localhost:888", Options{
		Retry: 0,
		TLS:   nil,
	})
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}
	defer client.End()
	if err = client.Head("/head"); err != nil {
		t.Log(err)
		t.Fail()
		return
	}
}

/// -------------------------------- HTTPS TESTING --------------------------------

// 测试Https GET请求
func TestHttpClient_GET_With_TLS(t *testing.T) {}

// 测试Https POST请求
func TestHttpClient_Post_With_TLS(t *testing.T) {}

// 测试Https HEAD请求
func TestHttpClient_Head_With_TLS(t *testing.T) {}
