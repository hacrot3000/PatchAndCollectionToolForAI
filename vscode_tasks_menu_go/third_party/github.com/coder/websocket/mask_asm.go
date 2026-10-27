//go:build amd64 || arm64

package websocket

func mask(b []byte, key uint32) uint32 {
	// TODO: Will enable in v1.9.0.
	return maskGo(b, key)
	/*
		if len(b) > 0 {
			return maskAsm(&b[0], len(b), key)
		}
		return key
	*/
}

