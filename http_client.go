package SimpleHttpClient

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
	Any        = "*/*"
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
const (
	BAD_REQUEST int = 400
)

// default configs
const DefTimeout = time.Second * 10

// Errors
var (
	ErrConnectingTimeout = errors.New("connecting timeout")
	ErrRequestFail       = errors.New("request fail")
	ErrWithoutTLSConfig  = errors.New("the tls config was nil")
	ErrRequestCanceled   = errors.New("the requesting was canceled in proactive")
	ErrRequestTimeout    = errors.New("the request waiting response timeout")
)

// HttpClient
// ⾄少⽀持 GET / POST / HEAD 三个HTTP Method
// ⽀持Timeout / WriteTimeout / ReadTimeout 三种超时配置
// ⽀持Header配置
// TODO ⽀持https访问
// TODO ⽀持 RFC 1867(https://tools.ietf.org/html/rfc1867)
// TODO ⽀持连接池
// 支持HTTP 1.1 Keepalive特性
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

type httpClient struct {
	debug     bool
	connected bool // 连接是否可用
	ctx       context.Context
	conn      net.Conn // 复用连接
	connErr   error    // 连接错误
	req       *Request // 当前请求
	options   *Options // 请求配置选项
	lock      *sync.Mutex
}

type Options struct {
	Retry uint16
	TLS   *tls.ConnectionState
}

func NewHttpClient(ctx context.Context, baseUrl string, opts Options) (c HttpClient, err error) {
	isHttps := strings.HasPrefix(baseUrl, "https")
	if isHttps && opts.TLS == nil {
		return c, ErrWithoutTLSConfig
	}
	// xxx.xxx...:123
	host := strings.Split(baseUrl, "/")[2]
	// 添加连接默认端口
	if !strings.Contains(host, ":") {
		host += If(isHttps, ":443", ":80").(string)
	}
	fmt.Println("connecting ->", host)
	client := httpClient{
		debug: ctx.Value("DEV") != nil,
		ctx:   ctx,
		req: &Request{
			Host:         host,
			Headers:      defaultHeaders(host),
			Timeout:      DefTimeout,
			ReadTimeout:  DefTimeout,
			WriteTimeout: DefTimeout,
		},
		options: &opts,
		lock:    &sync.Mutex{},
	}
	// TODO Https
	go func(hc *httpClient) {
		conn, err := net.DialTimeout("tcp", host, client.req.Timeout)
		if err != nil {
			hc.connErr = err
			fmt.Println("connect err:", err)
			return
		}
		hc.conn = conn
		hc.connected = true
		fmt.Println("connected <-", conn.RemoteAddr())
	}(&client)
	c = &client
	return
}

func (c *httpClient) SetHeader(key, value string) {
	c.lock.Lock()
	c.req.Headers[key] = value
	c.lock.Unlock()
}

func (c *httpClient) SetTimeout(timeout time.Duration) {
	c.lock.Lock()
	c.req.Timeout = timeout
	c.lock.Unlock()
}

func (c *httpClient) SetWriteTimeout(timeout time.Duration) {
	c.lock.Lock()
	c.req.WriteTimeout = timeout
	c.lock.Unlock()
}

func (c *httpClient) SetReadTimeout(timeout time.Duration) {
	c.lock.Lock()
	c.req.ReadTimeout = timeout
	c.lock.Unlock()
}

func (c *httpClient) Head(url string) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.req.Method = "HEAD"
	c.req.Url = getUrl(url)
	_, err := c.request()
	return err
}

func (c *httpClient) Get(url string) (string, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.req.Method = "GET"
	c.req.Url = getUrl(url)
	return c.request()
}

func (c *httpClient) Post(url string, body io.Reader) (string, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.req.Method = "POST"
	c.req.Url = getUrl(url)
	c.req.Body = body
	return c.request()
}

func (c *httpClient) End() {
	if c.conn != nil && c.connected {
		err := c.conn.Close()
		c.connected = false
		c.printLog("\nClose Conn Err:", err)
	}
}

// 执行请求，并根据请求状态作相关重试工作
func (c *httpClient) request() (string, error) {
	var res Response
	if err := c.checkConnection(); err != nil {
		if c.needRetry() {
			goto Retry
		}
		c.req.Retry = 0
		return "", If(c.connected, ErrRequestFail, err).(error)
	}
	c.req.Done = false
	res = c.do()
	if c.needRetry() && ((res.Error != nil && res.Error != ErrRequestCanceled) || res.Status >= BAD_REQUEST) {
		goto Retry
	}
	c.req.Done = true
	c.req.Retry = 0
	c.printLog(c.req.Method, "->", res.Status, res.Error)
	return res.Body, res.Error
Retry:
	c.req.Retry++
	c.printLog("\n△", c.req.Method, "Retrying, count ->", c.req.Retry)
	return c.request()
}

// 发起请求
func (c *httpClient) do() Response {
	resChan := make(chan Response, 1)
	defer close(resChan)
	go c.requesting(resChan)
	for {
		select {
		case <-time.After(c.req.Timeout):
			return Response{Status: BAD_REQUEST, Error: ErrRequestTimeout}
		case <-c.ctx.Done():
			return Response{Status: BAD_REQUEST, Error: ErrRequestCanceled}
		case res := <-resChan:
			c.printLog("\nResponse ▼\n", res, "\n\n----------------------------")
			return res
		}
	}
}

// 实际请求逻辑
func (c *httpClient) requesting(resChan chan Response) {
	var (
		res Response
		err error
	)
	now := time.Now()
	if err = c.conn.SetDeadline(now.Add(c.req.Timeout)); err != nil {
		goto ResponseErr
	}
	if err = c.conn.SetReadDeadline(now.Add(c.req.ReadTimeout)); err != nil {
		goto ResponseErr
	}
	if err = c.conn.SetWriteDeadline(now.Add(c.req.WriteTimeout)); err != nil {
		goto ResponseErr
	}
	if err = c.sendRequestData(); err != nil {
		goto ResponseErr
	}
	if res, err = c.waitResponse(); err != nil {
		goto ResponseErr
	}
	resChan <- res
	return
ResponseErr:
	if !c.req.Done {
		resChan <- Response{Status: BAD_REQUEST, Error: err}
	}
}

// 发送请求数据
func (c *httpClient) sendRequestData() (err error) {
	reqBytes, err := c.req.GetRequestData()
	if err != nil {
		return
	}
	if _, err = c.conn.Write(reqBytes); err != nil {
		return
	}
	c.printLog("\n----------------------------\n", "Request ▼\n", string(reqBytes))
	return
}

// 等待响应消息
func (c *httpClient) waitResponse() (res Response, err error) {
	var totalBuf []byte
	for !c.req.Done {
		select {
		case <-c.ctx.Done():
			return
		default:
		}
		buf := make([]byte, 1024)
		// TODO 支持Accept-Encoding(压缩格式)
		cnt, err := c.conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				return Response{Status: BAD_REQUEST, Error: err}, err
			}
			break
		}
		totalBuf = append(totalBuf, buf[:cnt]...)
		if res = c.parseResponse(totalBuf); res.ContentLength == uint64(len(res.Body)) {
			// TODO 模拟较长的请求时间,在Debug下有效
			if c.debug {
				time.Sleep(2 * time.Second)
			}
			break
		}
	}
	return
}

// 解析响应报文
func (c *httpClient) parseResponse(originRes []byte) (res Response) {
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
func (c *httpClient) checkConnection() error {
	deadline := time.Now().Add(c.req.Timeout)
	for !c.connected {
		if time.Now().After(deadline) || c.connErr != nil {
			return ErrConnectingTimeout
		}
		runtime.Gosched()
	}
	return nil
}

// 判断是否需要进行重试
func (c *httpClient) needRetry() bool {
	return c.options.Retry > 0 && c.req.Retry < c.options.Retry
}

// 日志打印
func (c *httpClient) printLog(args ...interface{}) {
	if c.debug {
		fmt.Println(args...)
	}
}

// 生成默认的请求头
func defaultHeaders(host string) Headers {
	return Headers{
		"Host":            host,
		"User-Agent":      fmt.Sprintf("Simple_HTTP/1.0 (%s; CPU %s)", runtime.GOOS, runtime.GOARCH),
		"Accept":          Any,
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
