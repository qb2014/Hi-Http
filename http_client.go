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
	DefTimeout   = time.Second * 10
)

// HttpClient
// TODO ⾄少⽀持 GET / POST / HEAD 三个HTTP Method
// TODO ⽀持Timeout / WriteTimeout / ReadTimeout 三种超时配置
// ⽀持Header配置
// TODO ⽀持https访问
// TODO ⽀持 RFC 1867(https://tools.ietf.org/html/rfc1867)
// TODO ⽀持连接池
// TODO HTTP 1.1 需要支持Keepalive特性
type HttpClient interface {
	SetHeader(key, value string)
	SetTimeout(timeout time.Duration)
	SetWriteTimeout(timeout time.Duration)
	SetReadTimeout(timeout time.Duration)
	Get(url string) (string, error)
	Head(url string) error
	Post(url string, body io.Reader) (string, error)
}

type httpClient struct {
	req Request
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

func NewClient(req Request) HttpClient {
	return &httpClient{req}
}

func (client *httpClient) Get(url string) (string, error) {
	client.req.Method = "GET"
	client.req.Host = strings.Split(url, "/")[2]
	client.req.Url = url
	res, err := client.req.do()
	if err != nil {
		return "", err
	}
	return res.Body, nil
}

func (client *httpClient) Head(url string) error {
	return nil
}

func (client *httpClient) Post(url string, body io.Reader) (string, error) {
	client.req.Method = "POST"
	client.req.Host = strings.Split(url, "/")[2]
	client.req.Url = url

	//result := bytes.NewBuffer(nil)
	//var buf [65542]byte // 由于 标识数据包长度 的只有两个字节 故数据包最大为 2^16+4(魔数)+2(长度标识)
	//for {
	//	n, err := conn.Read(buf[0:])
	//	result.Write(buf[0:n])
	//	if err != nil {
	//		if err == io.EOF {
	//			continue
	//		} else {
	//			fmt.Println("read err:", err)
	//			break
	//		}
	//	} else {
	//		scanner := bufio.NewScanner(result)
	//		scanner.Split(packetSlitFunc)
	//		for scanner.Scan() {
	//			fmt.Println("recv:", string(scanner.Bytes()[6:]))
	//		}
	//	}
	//	result.Reset()
	//}

	buf := make([]byte, 1024)
	for {
		_, err := body.Read(buf)
		if err == io.EOF {
			break
		}
	}
	client.req.Body = buf
	client.SetHeader("Host", client.req.Host)
	client.SetHeader("Content-Length", strconv.Itoa(len(client.req.Body)))
	res, err := client.req.do()
	if err != nil {
		return "", err
	}
	return res.Body, nil
}

func (client *httpClient) SetHeader(key, value string) {
	client.req.Headers[key] = value
}

func (client *httpClient) SetTimeout(timeout time.Duration) {
	client.req.Timeout = timeout
}

func (client *httpClient) SetWriteTimeout(timeout time.Duration) {
	client.req.WriteTimeout = timeout
}

func (client *httpClient) SetReadTimeout(timeout time.Duration) {
	client.req.ReadTimeout = timeout
}

// 生成默认的请求头
func _DefaultHeaders() map[string]string {
	return map[string]string{
		// TODO 读取电脑信息
		"User-Agent":      "Mozilla/5.0 (iPhone; CPU iPhone OS 13_2_3 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/13.0.3 Mobile/15E148 Safari/604.1",
		"Content-type":    string(PLAIN),
		"Accept-Charset":  "UTF-8",
		"Accept-Language": "zh",
		"Accept-Encoding": "identity",
		"Content-Length":  "0",
		"Host":            "",
		"Date":            time.Now().String(),
	}
}
