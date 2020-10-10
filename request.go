package SimpleHttpClient

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

type Request struct {
	Host         string
	Method       string
	Url          string
	Body         []byte
	Headers      map[string]string
	Timeout      time.Duration
	WriteTimeout time.Duration
	ReadTimeout  time.Duration
	Options      Options
	Config       SSLConfig
}

// 生成请求报文
// TODO 计算Content-Length、设置默认Content-type、User-Agent、Accept-Charset、Accept-Language、Accept-Encoding(支持的压缩格式)
// TODO Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*
func (req *Request) _GenRequestData() []byte {
	reqBuilder := strings.Builder{}
	reqBuilder.WriteString(req.Method)
	reqBuilder.WriteString(" ")
	reqBuilder.WriteString(req.Url)
	reqBuilder.WriteString(" ")
	reqBuilder.WriteString(_HttpVersion)
	reqBuilder.WriteString("\r\n")
	for key, val := range req.Headers {
		reqBuilder.WriteString(key)
		reqBuilder.WriteString(":")
		reqBuilder.WriteString(val)
		reqBuilder.WriteString("\r\n")
	}
	reqBuilder.WriteString("\r\n")
	reqBuilder.WriteString(string(req.Body))
	return []byte(reqBuilder.String())
}

// TODO: 建立链接,读取数据长度并对数据包进行合并
func (req *Request) do() (res Response, err error) {
	conn, err := net.DialTimeout("tcp", req.Host, req.Timeout)
	if conn == nil || err != nil {
		return
	}
	defer conn.Close()
	now := time.Now()
	if err = conn.SetDeadline(now.Add(req.Timeout)); err != nil {
		return
	}
	if err = conn.SetReadDeadline(now.Add(req.ReadTimeout)); err != nil {
		return
	}
	if err = conn.SetWriteDeadline(now.Add(req.WriteTimeout)); err != nil {
		return
	}
	if stat, err := conn.Write(req._GenRequestData()); err != nil || stat == 0 {
		return
	}
	// TODO 是否足够缓存
	buf := make([]byte, 3072)
	for {
		cnt, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			} else {
				return
			}
		}
		if cnt == 0 {
			return
		}
		res, err = req._ParseResponseData(buf)
		fmt.Println(res.Headers)
		lengthStr, ok := res.Headers["Content-Length"]
		if !ok {
			continue
		}
		contLen, err := strconv.Atoi(lengthStr)
		if err != nil {
			return
		}
		if contLen == 0 || contLen <= len(buf) {
			break
		}
	}
	fmt.Printf("Connection from %v closed. \n", conn.RemoteAddr())
	return
}

// 解析响应报文
// TODO 处理Accept-Encoding(压缩格式)
func (req *Request) _ParseResponseData(originResData []byte) (res Response, err error) {
	res = Response{}
	resData := strings.Split(string(originResData), "\r\n")
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
	// 通过判断倒数第二行是否为空行来判断是否有Body
	if resData[dataPartCount-2] == "" {
		res.Body = resData[dataPartCount-1]
	}
	return
}
