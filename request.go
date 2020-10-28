package HiHttp

import (
	"io"
	"io/ioutil"
	"strconv"
	"time"
)

var (
	_HttpVerBytes = []byte("HTTP/1.1")
	_BrBytes      = []byte("\r\n")
)

type Headers map[string]string

type Request struct {
	Done        bool      // 标记请求是否已结束
	Retry       uint16    // 失败重试次数
	Method, Url string    // 请求方法、请求路径
	Headers     Headers   // 请求头信息
	Body        io.Reader // 请求体
	// 连接超时、写入超时、读取超时
	Timeout, WriteTimeout, ReadTimeout time.Duration
}

// 生成请求报文
func (req *Request) GetRequestData() (reqBytes []byte, err error) {
	reqBytes = append(reqBytes, []byte(req.Method)...)
	// 空格: 0x20
	reqBytes = append(reqBytes, 0x20)
	reqBytes = append(reqBytes, []byte(req.Url)...)
	reqBytes = append(reqBytes, 0x20)
	reqBytes = append(reqBytes, _HttpVerBytes...)
	reqBytes = append(reqBytes, _BrBytes...)
	var bodyBytes []byte
	if req.Body != nil {
		bodyBytes, err = ioutil.ReadAll(req.Body)
		if err != nil {
			return
		}
		req.Headers["Content-Length"] = strconv.Itoa(len(bodyBytes))
	}
	for key, val := range req.Headers {
		reqBytes = append(reqBytes, []byte(key)...)
		// : + 空格: 0x3A, 0x20
		reqBytes = append(reqBytes, 0x3A, 0x20)
		reqBytes = append(reqBytes, []byte(val)...)
		reqBytes = append(reqBytes, _BrBytes...)
	}
	reqBytes = append(reqBytes, _BrBytes...)
	if len(bodyBytes) > 0 {
		reqBytes = append(reqBytes, bodyBytes...)
	}
	return
}
