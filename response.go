package HiHttp

type Response struct {
	Version       string
	Status        int
	Description   string
	Headers       Headers
	Error         error
	ContentLength uint64
	Body          string
}
