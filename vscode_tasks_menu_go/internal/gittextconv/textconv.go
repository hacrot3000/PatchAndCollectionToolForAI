package gittextconv

import (
	"io"
	"os"
)

// Normalize converts only standalone carriage returns (CR-only line endings) to
// LF. Existing CRLF pairs are preserved byte-for-byte, so normal Windows files
// keep their original line endings while legacy CR-only text becomes line based
// for Git's textconv diff pipeline.
func Normalize(r io.Reader, w io.Writer) error {
	buf := make([]byte, 64*1024)
	out := make([]byte, 0, len(buf)+1)
	pendingCR := false

	for {
		n, err := r.Read(buf)
		if n > 0 {
			out = out[:0]
			for _, b := range buf[:n] {
				if pendingCR {
					if b == '\n' {
						out = append(out, '\r', '\n')
						pendingCR = false
						continue
					}
					out = append(out, '\n')
					pendingCR = false
				}
				if b == '\r' {
					pendingCR = true
					continue
				}
				out = append(out, b)
			}
			if len(out) > 0 {
				if _, writeErr := w.Write(out); writeErr != nil {
					return writeErr
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	if pendingCR {
		_, err := w.Write([]byte{'\n'})
		return err
	}
	return nil
}

func NormalizeFile(path string, w io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return Normalize(f, w)
}
