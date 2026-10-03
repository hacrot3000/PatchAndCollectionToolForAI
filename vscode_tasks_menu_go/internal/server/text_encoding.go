package server

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

type decodedText struct {
	Text     string
	Encoding string
	BOM      bool
	Lossy    bool
}

func decodeMarkdownText(data []byte) (decodedText, error) {
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		body := data[3:]
		if !utf8.Valid(body) {
			return decodedText{}, fmt.Errorf("invalid UTF-8 Markdown")
		}
		return decodedText{Text: string(body), Encoding: "utf-8", BOM: true}, nil
	}
	if utf8.Valid(data) && bytes.IndexByte(data, 0) < 0 {
		return decodedText{Text: string(data), Encoding: "utf-8"}, nil
	}
	if bytes.HasPrefix(data, []byte{0xFF, 0xFE}) {
		text, err := decodeUTF16Bytes(data[2:], true)
		if err != nil {
			return decodedText{}, err
		}
		return decodedText{Text: text, Encoding: "utf-16le", BOM: true}, nil
	}
	if bytes.HasPrefix(data, []byte{0xFE, 0xFF}) {
		text, err := decodeUTF16Bytes(data[2:], false)
		if err != nil {
			return decodedText{}, err
		}
		return decodedText{Text: text, Encoding: "utf-16be", BOM: true}, nil
	}
	if little, ok := likelyUTF16(data); ok {
		text, err := decodeUTF16Bytes(data, little)
		if err == nil {
			encoding := "utf-16be"
			if little {
				encoding = "utf-16le"
			}
			return decodedText{Text: text, Encoding: encoding}, nil
		}
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return decodedText{}, fmt.Errorf("Markdown contains binary NUL bytes")
	}
	return decodedText{Text: decodeWindows1252(data), Encoding: "windows-1252", Lossy: true}, nil
}

func decodeUTF16Bytes(data []byte, little bool) (string, error) {
	if len(data) == 0 {
		return "", nil
	}
	if len(data)%2 != 0 {
		data = data[:len(data)-1]
	}
	units := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		var value uint16
		if little {
			value = uint16(data[i]) | uint16(data[i+1])<<8
		} else {
			value = uint16(data[i])<<8 | uint16(data[i+1])
		}
		units = append(units, value)
	}
	return string(utf16.Decode(units)), nil
}

func likelyUTF16(data []byte) (little bool, ok bool) {
	if len(data) < 8 {
		return false, false
	}
	limit := len(data)
	if limit > 4096 {
		limit = 4096
	}
	evenZero, oddZero, pairs := 0, 0, 0
	for i := 0; i+1 < limit; i += 2 {
		pairs++
		if data[i] == 0 {
			evenZero++
		}
		if data[i+1] == 0 {
			oddZero++
		}
	}
	if pairs == 0 {
		return false, false
	}
	if oddZero*100/pairs >= 35 && evenZero*100/pairs <= 10 {
		return true, true
	}
	if evenZero*100/pairs >= 35 && oddZero*100/pairs <= 10 {
		return false, true
	}
	return false, false
}

func decodeWindows1252(data []byte) string {
	var b strings.Builder
	b.Grow(len(data))
	for _, value := range data {
		if value < 0x80 || value >= 0xA0 {
			b.WriteRune(rune(value))
			continue
		}
		switch value {
		case 0x80: b.WriteRune('€')
		case 0x82: b.WriteRune('‚')
		case 0x83: b.WriteRune('ƒ')
		case 0x84: b.WriteRune('„')
		case 0x85: b.WriteRune('…')
		case 0x86: b.WriteRune('†')
		case 0x87: b.WriteRune('‡')
		case 0x88: b.WriteRune('ˆ')
		case 0x89: b.WriteRune('‰')
		case 0x8A: b.WriteRune('Š')
		case 0x8B: b.WriteRune('‹')
		case 0x8C: b.WriteRune('Œ')
		case 0x8E: b.WriteRune('Ž')
		case 0x91: b.WriteRune('‘')
		case 0x92: b.WriteRune('’')
		case 0x93: b.WriteRune('“')
		case 0x94: b.WriteRune('”')
		case 0x95: b.WriteRune('•')
		case 0x96: b.WriteRune('–')
		case 0x97: b.WriteRune('—')
		case 0x98: b.WriteRune('˜')
		case 0x99: b.WriteRune('™')
		case 0x9A: b.WriteRune('š')
		case 0x9B: b.WriteRune('›')
		case 0x9C: b.WriteRune('œ')
		case 0x9E: b.WriteRune('ž')
		case 0x9F: b.WriteRune('Ÿ')
		default:
			b.WriteRune('�')
		}
	}
	return b.String()
}
