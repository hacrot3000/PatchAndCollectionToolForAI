package dbmysql

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

func normalizeSingleStatement(statement string) (string, error) {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return "", errors.New("MySQL statement is required")
	}

	var (
		quote        rune
		blockComment bool
		lineComment  bool
		escaped      bool
		semicolon    = -1
	)
	runes := []rune(statement)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}

		if lineComment {
			if r == '\n' || r == '\r' {
				lineComment = false
			}
			continue
		}
		if blockComment {
			if r == '*' && next == '/' {
				blockComment = false
				i++
			}
			continue
		}
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' && quote != '`' {
				escaped = true
				continue
			}
			if r == quote {
				if next == quote {
					i++
					continue
				}
				quote = 0
			}
			continue
		}

		switch {
		case r == '\'' || r == '"' || r == '`':
			quote = r
		case r == '#':
			lineComment = true
		case r == '-' && next == '-' && (i+2 >= len(runes) || unicode.IsSpace(runes[i+2])):
			lineComment = true
			i++
		case r == '/' && next == '*':
			blockComment = true
			i++
		case r == ';':
			if semicolon >= 0 {
				return "", errors.New("MySQL execute accepts exactly one statement")
			}
			semicolon = i
		}
	}
	if quote != 0 {
		return "", errors.New("MySQL statement has an unterminated quoted value")
	}
	if blockComment {
		return "", errors.New("MySQL statement has an unterminated block comment")
	}
	if semicolon >= 0 {
		for _, r := range runes[semicolon+1:] {
			if !unicode.IsSpace(r) {
				return "", errors.New("MySQL execute accepts exactly one statement")
			}
		}
		statement = strings.TrimSpace(string(runes[:semicolon]))
		if statement == "" {
			return "", errors.New("MySQL statement is required")
		}
	}
	return statement, nil
}

func readOnlyStatement(statement string) error {
	normalized, err := normalizeSingleStatement(statement)
	if err != nil {
		return err
	}
	if containsMySQLExecutableComment(normalized) {
		return errors.New("MySQL read-only profile rejects executable comments")
	}
	keyword := firstSQLKeyword(normalized)
	switch keyword {
	case "SELECT", "SHOW", "DESCRIBE", "DESC", "EXPLAIN":
	default:
		return fmt.Errorf("MySQL read-only profile does not allow %q statements", keyword)
	}
	if keyword == "SELECT" {
		upper := strings.ToUpper(stripSQLQuotedAndComments(normalized))
		for _, forbidden := range []string{" INTO OUTFILE", " INTO DUMPFILE", " FOR UPDATE", " LOCK IN SHARE MODE"} {
			if strings.Contains(" "+upper, forbidden) {
				return fmt.Errorf("MySQL read-only profile rejects %s", strings.TrimSpace(forbidden))
			}
		}
	}
	return nil
}

func containsMySQLExecutableComment(statement string) bool {
	runes := []rune(statement)
	var (
		quote   rune
		escaped bool
	)
	for i := 0; i+2 < len(runes); i++ {
		r := runes[i]
		next := runes[i+1]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' && quote != '`' {
				escaped = true
				continue
			}
			if r == quote {
				if i+1 < len(runes) && runes[i+1] == quote {
					i++
					continue
				}
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' || r == '`' {
			quote = r
			continue
		}
		if r == '/' && next == '*' {
			if runes[i+2] == '!' {
				return true
			}
			if (runes[i+2] == 'M' || runes[i+2] == 'm') && i+3 < len(runes) && runes[i+3] == '!' {
				return true
			}
		}
	}
	return false
}

func wrapReadOnlyStatement(statement string) string {
	return "START TRANSACTION READ ONLY;\n" + statement + ";\nROLLBACK"
}

func firstSQLKeyword(statement string) string {
	clean := strings.TrimSpace(stripLeadingSQLComments(statement))
	end := 0
	for end < len(clean) {
		c := clean[end]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_' {
			end++
			continue
		}
		break
	}
	if end == 0 {
		return ""
	}
	return strings.ToUpper(clean[:end])
}

func stripLeadingSQLComments(statement string) string {
	s := strings.TrimSpace(statement)
	for {
		switch {
		case strings.HasPrefix(s, "#"):
			if index := strings.IndexByte(s, '\n'); index >= 0 {
				s = strings.TrimSpace(s[index+1:])
				continue
			}
			return ""
		case strings.HasPrefix(s, "--") && (len(s) == 2 || unicode.IsSpace(rune(s[2]))):
			if index := strings.IndexByte(s, '\n'); index >= 0 {
				s = strings.TrimSpace(s[index+1:])
				continue
			}
			return ""
		case strings.HasPrefix(s, "/*"):
			index := strings.Index(s[2:], "*/")
			if index < 0 {
				return s
			}
			s = strings.TrimSpace(s[index+4:])
			continue
		default:
			return s
		}
	}
}

func stripSQLQuotedAndComments(statement string) string {
	var builder strings.Builder
	runes := []rune(statement)
	var (
		quote        rune
		blockComment bool
		lineComment  bool
		escaped      bool
	)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}
		if lineComment {
			if r == '\n' || r == '\r' {
				lineComment = false
				builder.WriteRune(' ')
			}
			continue
		}
		if blockComment {
			if r == '*' && next == '/' {
				blockComment = false
				i++
				builder.WriteRune(' ')
			}
			continue
		}
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' && quote != '`' {
				escaped = true
				continue
			}
			if r == quote {
				if next == quote {
					i++
					continue
				}
				quote = 0
				builder.WriteRune(' ')
			}
			continue
		}
		switch {
		case r == '\'' || r == '"' || r == '`':
			quote = r
			builder.WriteRune(' ')
		case r == '#':
			lineComment = true
			builder.WriteRune(' ')
		case r == '-' && next == '-' && (i+2 >= len(runes) || unicode.IsSpace(runes[i+2])):
			lineComment = true
			i++
			builder.WriteRune(' ')
		case r == '/' && next == '*':
			blockComment = true
			i++
			builder.WriteRune(' ')
		default:
			builder.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}
