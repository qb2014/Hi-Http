package HiHttp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ContentType
const (
	ANY        = "*/*"
	JSON       = "application/json"
	HTML       = "text/html"
	XML        = "application/xml"
	XML2       = "text/xml"
	PLAIN      = "text/plain"
	URLENCODED = "application/x-www-form-urlencoded"
	MULTIPART  = "multipart/form-data"
	PROTOBUF   = "application/x-protobuf"
	MSGPACK    = "application/x-msgpack"
	MSGPACK2   = "application/msgpack"
)

// Status code
const BAD_REQUEST = 400

// default configs
const DefTimeout = time.Second * 10

// Errors
var (
	ErrConnectingTimeout = errors.New("connecting timeout")
	ErrRequestFail       = errors.New("request fail")
	ErrRequestCanceled   = errors.New("the requesting was canceled in proactive")
	ErrRequestTimeout    = errors.New("the request waiting response timeout")
)

// HttpClient
// ⽀持 GET / POST / HEAD Method
// ⽀持Timeout / WriteTimeout / ReadTimeout 三种超时配置
// ⽀持Header配置
// 支持HTTP 1.1 Keepalive特性
// TODO ⽀持HTTPS访问
// TODO ⽀持 RFC 1867(https://tools.ietf.org/html/rfc1867) Multipart Form
// TODO ⽀持连接池
type HttpClient interface {
	SetHeader(key, value string)
	SetTimeout(timeout time.Duration)
	SetWriteTimeout(timeout time.Duration)
	SetReadTimeout(timeout time.Duration)
	Get(url string) (string, error)
	Head(url string) error
	Post(url string, body io.Reader) (string, error)
	// 释放连接
	End()
}

type hiHttp struct {
	debug     bool // 标志是否为调试模式
	ctx       context.Context
	host      string               // 主机
	connected bool                 // 连接是否可用
	conn      net.Conn             // 复用连接
	connErr   error                // 连接错误
	tls       *tls.ConnectionState // TLS配置
	req       *Request             // 当前请求
	options   *Options             // 请求配置选项
	lock      *sync.Mutex
}

/// 客户端配置
type Options struct {
	PoolSize, IdleCount uint16 // 连接池最大容量、连接池最少连接存活的数量
	Retry               uint16 // 请求失败重试次数
}

func HiHttp(ctx context.Context, baseUrl string, opts Options) (c HttpClient, err error) {
	isHttps := strings.HasPrefix(baseUrl, "https")
	// xxx.xxx...:123
	host := strings.Split(baseUrl, "/")[2]
	// 添加默认连接端口
	if !strings.Contains(host, ":") {
		host += If(isHttps, ":443", ":80").(string)
	}
	client := hiHttp{
		debug: ctx.Value("DEV") != nil,
		ctx:   ctx,
		host:  host,
		req: &Request{
			Headers:      defaultHeaders(host),
			Timeout:      DefTimeout,
			ReadTimeout:  DefTimeout,
			WriteTimeout: DefTimeout,
		},
		options: &opts,
		lock:    &sync.Mutex{},
	}
	client.printLog("connecting ->", host)
	go If(isHttps, client.dialWithTLS, client.dial).(func())()
	c = &client
	return
}

func (h *hiHttp) dial() {
	conn, err := net.DialTimeout("tcp", h.host, h.req.Timeout)
	if err != nil {
		h.connErr = err
		h.printLog("connect err:", err)
		return
	}
	h.conn = conn
	h.connected = true
	h.printLog("connected <-", conn.RemoteAddr())
}

// TODO 实现HTTPS握手
func (h *hiHttp) dialWithTLS() {
	conn, err := net.DialTimeout("tcp", h.host, h.req.Timeout)
	if err != nil {
		h.connErr = err
		h.printLog("connect err:", err)
		return
	}
	if _, err := conn.Write(getClientHello()); err != nil {
		h.connErr = err
		conn.Close()
		h.printLog("send Hello err:", err)
		return
	}
	if resultBytes, err := readSocketData(conn); err != nil {
		h.connErr = err
		conn.Close()
		h.printLog("read Hello err:", err)
		return
	} else {
		h.printLog("server hello ->", resultBytes)
	}
	if resultBytes, err := readSocketData(conn); err != nil {
		h.connErr = err
		conn.Close()
		h.printLog("read Certificate err:", err)
		return
	} else {
		h.printLog("server Certificate ->", resultBytes)
	}
	h.conn = conn
	h.connected = true
	h.printLog("connected <-", conn.RemoteAddr())
}

func (h *hiHttp) SetHeader(key, value string) {
	h.lock.Lock()
	h.req.Headers[key] = value
	h.lock.Unlock()
}

func (h *hiHttp) SetTimeout(timeout time.Duration) {
	h.lock.Lock()
	h.req.Timeout = timeout
	h.lock.Unlock()
}

func (h *hiHttp) SetWriteTimeout(timeout time.Duration) {
	h.lock.Lock()
	h.req.WriteTimeout = timeout
	h.lock.Unlock()
}

func (h *hiHttp) SetReadTimeout(timeout time.Duration) {
	h.lock.Lock()
	h.req.ReadTimeout = timeout
	h.lock.Unlock()
}

func (h *hiHttp) Head(url string) error {
	h.lock.Lock()
	defer h.lock.Unlock()
	h.req.Method = "HEAD"
	h.req.Url = getUrl(url)
	_, err := h.request()
	return err
}

func (h *hiHttp) Get(url string) (string, error) {
	h.lock.Lock()
	defer h.lock.Unlock()
	h.req.Method = "GET"
	h.req.Url = getUrl(url)
	return h.request()
}

func (h *hiHttp) Post(url string, body io.Reader) (string, error) {
	h.lock.Lock()
	defer h.lock.Unlock()
	h.req.Method = "POST"
	h.req.Url = getUrl(url)
	h.req.Body = body
	return h.request()
}

func (h *hiHttp) End() {
	if h.conn != nil && h.connected {
		err := h.conn.Close()
		h.connected = false
		h.printLog("\nClose Conn Err:", err)
	}
}

// 执行请求，并根据请求状态作相关重试工作
func (h *hiHttp) request() (string, error) {
	var res Response
	if err := h.checkConnection(); err != nil {
		if h.needRetry() {
			goto Retry
		}
		h.req.Retry = 0
		return res.Body, If(h.connected, ErrRequestFail, err).(error)
	}
	if h.req.Done {
		h.req.Done = false
	}
	res = h.do()
	if h.needRetry() && ((res.Error != nil && res.Error != ErrRequestCanceled) || res.Status >= BAD_REQUEST) {
		goto Retry
	}
	if !h.req.Done {
		h.req.Done = true
	}
	h.req.Retry = 0
	if res.Status >= BAD_REQUEST && res.Error == nil {
		res.Error = ErrRequestFail
	}
	h.printLog(h.req.Method, "->", res.Status, res.Error)
	return res.Body, res.Error
Retry:
	h.req.Retry++
	h.printLog("\n△", h.req.Method, "Retrying, count ->", h.req.Retry)
	return h.request()
}

// 发起请求
func (h *hiHttp) do() Response {
	resChan := make(chan Response, 1)
	defer close(resChan)
	go h.requesting(resChan)
	for {
		select {
		case <-time.After(h.req.Timeout):
			return Response{Status: BAD_REQUEST, Error: ErrRequestTimeout}
		case <-h.ctx.Done():
			return Response{Status: BAD_REQUEST, Error: ErrRequestCanceled}
		case res := <-resChan:
			h.printLog("\nResponse ▼\n", res, "\n\n----------------------------")
			return res
		}
	}
}

// 实际请求逻辑
func (h *hiHttp) requesting(resChan chan Response) {
	var (
		res Response
		err error
	)
	now := time.Now()
	if err = h.conn.SetDeadline(now.Add(h.req.Timeout)); err != nil {
		goto ResponseErr
	}
	if err = h.conn.SetReadDeadline(now.Add(h.req.ReadTimeout)); err != nil {
		goto ResponseErr
	}
	if err = h.conn.SetWriteDeadline(now.Add(h.req.WriteTimeout)); err != nil {
		goto ResponseErr
	}
	if err = h.sendRequestData(); err != nil {
		goto ResponseErr
	}
	if res, err = h.waitResponse(); err != nil {
		goto ResponseErr
	}
	resChan <- res
	return
ResponseErr:
	if !h.req.Done {
		resChan <- Response{Status: BAD_REQUEST, Error: err}
	}
}

// 发送请求数据
func (h *hiHttp) sendRequestData() (err error) {
	reqBytes, err := h.req.GetRequestData()
	if err != nil {
		return
	}
	if _, err = h.conn.Write(reqBytes); err != nil {
		return
	}
	h.printLog("\n----------------------------\n", "Request ▼\n", string(reqBytes))
	return
}

// 等待响应消息
func (h *hiHttp) waitResponse() (res Response, err error) {
	var totalBuf []byte
	for !h.req.Done {
		select {
		case <-h.ctx.Done():
			return
		default:
		}
		buf := make([]byte, 1024)
		// TODO 支持Accept-Encoding(压缩格式)
		cnt, err := h.conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				return Response{Status: BAD_REQUEST, Error: err}, err
			}
			break
		}
		totalBuf = append(totalBuf, buf[:cnt]...)
		if res = h.parseResponse(totalBuf); res.ContentLength == uint64(len(res.Body)) {
			// TODO 模拟较长的请求时间,在Debug下有效
			if h.debug {
				time.Sleep(2 * time.Second)
			}
			break
		}
	}
	return
}

// 解析响应报文
func (h *hiHttp) parseResponse(originRes []byte) (res Response) {
	if len(originRes) == 0 {
		return
	}
	resData := strings.Split(string(originRes), "\r\n")
	partCount := len(resData)
	statusInfos := strings.Split(resData[0], " ")
	res.Version = statusInfos[0]
	status, err := strconv.Atoi(statusInfos[1])
	if err != nil {
		res.Status = BAD_REQUEST
		res.Error = err
		return
	}
	res.Status = status
	res.Description = strings.Join(statusInfos[2:], " ")
	// 截取报文中的Header部分
	header := resData[1:If(resData[partCount-2] == "", partCount-2, partCount).(int)]
	res.Headers = make(map[string]string, len(header))
	for _, head := range header {
		headInfo := strings.Split(head, ": ")
		if len(headInfo) == 2 {
			res.Headers[headInfo[0]] = headInfo[1]
		}
	}
	cl, err := parseContentLength(res.Headers["Content-Length"])
	if err != nil {
		res.Status = BAD_REQUEST
		res.Error = err
		return
	}
	res.ContentLength = cl
	if cl > 0 {
		res.Body = resData[partCount-1]
	}
	return
}

// 检查与服务器的连接是否成功，若在指定时间内未成功则会返回连接超时错误
func (h *hiHttp) checkConnection() error {
	deadline := time.Now().Add(h.req.Timeout)
	for !h.connected {
		if time.Now().After(deadline) || h.connErr != nil {
			return ErrConnectingTimeout
		}
		runtime.Gosched()
	}
	return nil
}

// 判断是否需要进行重试
func (h *hiHttp) needRetry() bool {
	return h.options.Retry > 0 && h.req.Retry < h.options.Retry
}

// 日志打印
func (h *hiHttp) printLog(args ...interface{}) {
	if h.debug {
		fmt.Println(args...)
	}
}

// 生成默认的请求头
func defaultHeaders(host string) Headers {
	return Headers{
		"Host":            host,
		"User-Agent":      fmt.Sprintf("Simple_HTTP/1.0 (%s; CPU %s)", runtime.GOOS, runtime.GOARCH),
		"Accept":          ANY,
		"Accept-Charset":  "utf-8",
		"Accept-Language": "zh",
		"Accept-Encoding": "deflate",
		"Cache-Control":   "no-cache",
		"Connection":      "keep-alive",
	}
}

// 解析并获取
func getUrl(url string) string {
	if strings.HasPrefix(url, "http") {
		return "/" + strings.Join(strings.Split(url, "/")[3:], "/")
	}
	return url
}

// 解析ContentLength
func parseContentLength(cl string) (l uint64, err error) {
	if cl == "" {
		return 0, nil
	}
	return strconv.ParseUint(cl, 10, 63)
}

func readSocketData(conn net.Conn) (data []byte, err error) {
	for {
		buf := make([]byte, 256)
		cnt, err := conn.Read(buf)
		if err != nil {
			break
		}
		data = append(data, buf[:cnt]...)
	}
	return
}

// TODO
func getClientHello() []byte {
	return []byte{0x16, 0x03, 0x01, 0x02, 0x00, 0x01, 0x00, 0x01, 0xfc, 0x03, 0x03, 0xf8, 0x1d, 0x99, 0xa4, 0x55, 0x20, 0x3d, 0x8b, 0xc8, 0x72, 0xda, 0xf7, 0x68, 0x7b, 0x23, 0xb5, 0x36, 0x45, 0xe3, 0x00, 0x2f, 0xe7, 0xf7, 0x10, 0xf2, 0x99, 0x45, 0x62, 0x81, 0x98, 0x1a, 0x76, 0x20, 0xbe, 0x14, 0x37, 0xff, 0x1e, 0x71, 0x5a, 0x29, 0x40, 0x2f, 0x96, 0xfd, 0xf0, 0x8f, 0x9a, 0x39, 0x32, 0x19, 0xc6, 0x9d, 0xf6, 0xcd, 0x9f, 0x98, 0x0b, 0xe3, 0x6a, 0x73, 0xdd, 0x80, 0x34, 0x22, 0x00, 0x20, 0x7a, 0x7a, 0x13, 0x01, 0x13, 0x02, 0x13, 0x03, 0xc0, 0x2b, 0xc0, 0x2f, 0xc0, 0x2c, 0xc0, 0x30, 0xcc, 0xa9, 0xcc, 0xa8, 0xc0, 0x13, 0xc0, 0x14, 0x00, 0x9c, 0x00, 0x9d, 0x00, 0x2f, 0x00, 0x35, 0x01, 0x00, 0x01, 0x93, 0x8a, 0x8a, 0x00, 0x00, 0x00, 0x17, 0x00, 0x00, 0xff, 0x01, 0x00, 0x01, 0x00, 0x00, 0x0a, 0x00, 0x0a, 0x00, 0x08, 0x6a, 0x6a, 0x00, 0x1d, 0x00, 0x17, 0x00, 0x18, 0x00, 0x0b, 0x00, 0x02, 0x01, 0x00, 0x00, 0x23, 0x00, 0x00, 0x00, 0x10, 0x00, 0x0e, 0x00, 0x0c, 0x02, 0x68, 0x32, 0x08, 0x68, 0x74, 0x74, 0x70, 0x2f, 0x31, 0x2e, 0x31, 0x00, 0x05, 0x00, 0x05, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0d, 0x00, 0x12, 0x00, 0x10, 0x04, 0x03, 0x08, 0x04, 0x04, 0x01, 0x05, 0x03, 0x08, 0x05, 0x05, 0x01, 0x08, 0x06, 0x06, 0x01, 0x00, 0x12, 0x00, 0x00, 0x00, 0x33, 0x00, 0x2b, 0x00, 0x29, 0x6a, 0x6a, 0x00, 0x01, 0x00, 0x00, 0x1d, 0x00, 0x20, 0x64, 0xbd, 0x66, 0xa0, 0x4a, 0xb9, 0x44, 0xc2, 0x04, 0x1b, 0x77, 0xce, 0xee, 0x69, 0x96, 0xff, 0x51, 0x00, 0x71, 0xeb, 0xd5, 0x08, 0x40, 0xe5, 0xb1, 0x2e, 0x14, 0xce, 0xbe, 0xd6, 0x34, 0x4c, 0x00, 0x2d, 0x00, 0x02, 0x01, 0x01, 0x00, 0x2b, 0x00, 0x0b, 0x0a, 0x4a, 0x4a, 0x03, 0x04, 0x03, 0x03, 0x03, 0x02, 0x03, 0x01, 0x00, 0x1b, 0x00, 0x03, 0x02, 0x00, 0x02, 0xca, 0xca, 0x00, 0x01, 0x00, 0x00, 0x15, 0x00, 0x45, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x29, 0x00, 0x9c, 0x00, 0x77, 0x00, 0x71, 0x2c, 0x8b, 0xa6, 0x74, 0x85, 0x07, 0xfb, 0x13, 0x96, 0x90, 0x06, 0xf1, 0x87, 0x03, 0xba, 0xf7, 0x53, 0x89, 0x40, 0xdb, 0xa2, 0x41, 0x88, 0x12, 0xe9, 0x72, 0x80, 0x08, 0xbd, 0x3b, 0x86, 0x46, 0x24, 0x8a, 0x46, 0x88, 0xc8, 0xba, 0xd3, 0x83, 0xbc, 0xd5, 0x96, 0x98, 0x73, 0x79, 0x4d, 0xfb, 0x66, 0x3e, 0x5f, 0x3b, 0x91, 0x5a, 0x4a, 0x64, 0xf2, 0xbc, 0x72, 0x7c, 0xe9, 0x1c, 0xa8, 0xdb, 0x39, 0x00, 0xda, 0xd4, 0x94, 0x37, 0xa7, 0x5b, 0x81, 0xc1, 0xad, 0xe1, 0xf1, 0x5b, 0xd3, 0x31, 0x8d, 0x7b, 0xb8, 0xb9, 0x04, 0x4a, 0x85, 0x73, 0x38, 0xf4, 0x6e, 0x2b, 0x24, 0xc8, 0x39, 0xf7, 0x8c, 0x12, 0x77, 0x49, 0x48, 0x51, 0xaa, 0x4a, 0x08, 0xa8, 0x5b, 0x56, 0xb9, 0xdc, 0x04, 0x87, 0xfe, 0x00, 0x01, 0xdc, 0x90, 0x00, 0x21, 0x20, 0xb8, 0xa9, 0x61, 0xc6, 0xf5, 0x23, 0x90, 0xc4, 0xb5, 0x60, 0x12, 0x45, 0x95, 0x77, 0x57, 0xbd, 0x5e, 0x8e, 0x00, 0x65, 0x01, 0x02, 0xff, 0x44, 0xd8, 0x7d, 0x92, 0x7f, 0x58, 0x50, 0x90, 0xd6}
	//return []byte{0x16, 1, 0, 0, 0x11, 3, 1, 1, 2, 3, 4, 4, 5, 6, 7, 8, 4, 1, 1, 0, 1, 0}
}
