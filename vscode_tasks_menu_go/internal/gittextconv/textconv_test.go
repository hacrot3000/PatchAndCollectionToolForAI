package gittextconv

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestNormalizeStandaloneCRPreservesCRLF(t *testing.T) {
	input := "alpha\rbeta\r\ngamma\ndelta\r"
	var out bytes.Buffer
	if err := Normalize(strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	want := "alpha\nbeta\r\ngamma\ndelta\n"
	if out.String() != want {
		t.Fatalf("Normalize()=%q want %q", out.String(), want)
	}
}

type oneByteReader struct{ data []byte }

func (r *oneByteReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	p[0] = r.data[0]
	r.data = r.data[1:]
	return 1, nil
}

func TestNormalizePreservesCRLFAcrossReadBoundaries(t *testing.T) {
	r := &oneByteReader{data: []byte("a\r\nb\rc")}
	var out bytes.Buffer
	if err := Normalize(r, &out); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "a\r\nb\nc"; got != want {
		t.Fatalf("Normalize()=%q want %q", got, want)
	}
}
