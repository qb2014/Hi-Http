package SimpleHttpClient

func If(cond bool, then interface{}, els interface{}) interface{} {
	if cond {
		return then
	}
	return els
}
