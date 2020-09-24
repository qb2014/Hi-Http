package SimpleHttpClient

import (
	"io"
	"strconv"
	"strings"
	"time"
)

type ContentType string

// 常见的ContentType
const (
	JSON       ContentType = "application/json"
	HTML       ContentType = "text/html"
	XML        ContentType = "application/xml"
	XML2       ContentType = "text/xml"
	PLAIN      ContentType = "text/plain"
	URLENCODED ContentType = "application/x-www-form-urlencoded"
	MULTIPART  ContentType = "multipart/form-data"
	PROTOBUF   ContentType = "application/x-protobuf"
	MSGPACK    ContentType = "application/x-msgpack"
	MSGPACK2   ContentType = "application/msgpack"
)

const (
	_HttpVersion = "Http/1.1"
	_defTimeout  = time.Second * 10
)

// HttpClient
// TODO ⾄少⽀持 GET / POST / HEAD 三个HTTP Method
// TODO ⽀持Timeout / WriteTimeout / ReadTimeout 三种超时配置
// ⽀持Header配置
// TODO ⽀持https访问
type HttpClient interface {
	SetHeader(key, value string)
	SetTimeout(timeout time.Duration)
	SetWriteTimeout(timeout time.Duration)
	SetReadTimeout(timeout time.Duration)
	Get(url string) (string, error)
	Head(url string) error
	Post(url string, body io.Reader) (string, error)
}

type Request struct {
	_headers      map[string]string
	_timeout      time.Duration
	_writeTimeout time.Duration
	_readTimeout  time.Duration
	_options      Options
	_config       SSLConfig
}

type Response struct {
	Version     string
	Status      int
	Description string
	Headers     map[string]string
	Body        string
}

type Options struct {
	Retry uint16
}

type SSLConfig struct {
	SSLCert string
}

func New(opt Options) Request {
	return Request{
		_headers:      _DefaultHeaders(),
		_timeout:      _defTimeout,
		_readTimeout:  _defTimeout,
		_writeTimeout: _defTimeout,
		_options:      opt,
	}
}

func NewWithSSL(opt Options, ssl SSLConfig) Request {
	return Request{
		_headers:      _DefaultHeaders(),
		_timeout:      _defTimeout,
		_readTimeout:  _defTimeout,
		_writeTimeout: _defTimeout,
		_options:      opt,
		_config:       ssl,
	}
}

func (req *Request) Get(url string) (string, error) {
	return "", nil
}

func (req *Request) Head(url string) error {
	return nil
}

func (req *Request) Post(url string, body io.Reader) (string, error) {
	return "", nil
}

func (req *Request) SetHeader(key, value string) {
	req._headers[key] = value
}

func (req *Request) SetTimeout(timeout time.Duration) {
	req._timeout = timeout
}

func (req *Request) SetWriteTimeout(timeout time.Duration) {
	req._writeTimeout = timeout
}

func (req *Request) SetReadTimeout(timeout time.Duration) {
	req._readTimeout = timeout
}

// 生成请求报文
// TODO 计算Content-Length、设置默认Content-type、User-Agent、Accept-Charset、Accept-Language、Accept-Encoding(支持的压缩格式)
// TODO Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*
func (req *Request) _GenRequestData(method, url, body string) string {
	reqBuilder := strings.Builder{}
	reqBuilder.WriteString(method)
	reqBuilder.WriteString(" ")
	reqBuilder.WriteString(url)
	reqBuilder.WriteString(" ")
	reqBuilder.WriteString(_HttpVersion)
	reqBuilder.WriteString("\r\n")
	for key, val := range req._headers {
		reqBuilder.WriteString(key)
		reqBuilder.WriteString(":")
		reqBuilder.WriteString(val)
		reqBuilder.WriteString("\r\n")
	}
	reqBuilder.WriteString("\r\n")
	reqBuilder.WriteString(body)
	return reqBuilder.String()
}

// 解析响应报文
// TODO 处理Accept-Encoding(压缩格式)
func (req *Request) _ParseResponseData(originRes string) (res Response, err error) {
	res = Response{}
	resData := strings.Split(originRes, "\r\n")
	dataPartCount := len(resData)
	baseInfos := strings.Split(resData[0], " ")
	res.Version = baseInfos[0]
	res.Description = baseInfos[2]
	status, err := strconv.Atoi(baseInfos[1])
	if err != nil {
		return
	}
	res.Status = status
	// 截取报文中的Header部分
	heads := resData[1 : dataPartCount-2]
	res.Headers = make(map[string]string, len(heads))
	for _, head := range heads {
		headInfo := strings.Split(head, ":")
		res.Headers[headInfo[0]] = headInfo[1]
	}
	res.Body = resData[dataPartCount-1]
	return
}

// 生成默认的请求头
func _DefaultHeaders() map[string]string {
	return map[string]string{
		// TODO 读取电脑信息
		"User-Agent":      "",
		"Content-type":    string(PLAIN),
		"Accept-Charset":  "UTF-8",
		"Accept-Language": "zh",
		"Accept-Encoding": "compress, gzip",
		"Content-Length":  "",
		"Host":            "",
		"Date":            time.Now().String(),
	}
}
