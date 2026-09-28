package dbmysql

import (
	"fmt"
	"strings"
	"unicode"
)

type editableSelectTarget struct {
	Catalog   string
	Table     string
	Alias     string
	SelectAll bool
	Columns   []string
}

type sqlToken struct {
	Text  string
	Upper string
	Kind  string
	Depth int
}

func analyzeEditableSelect(statement string) (editableSelectTarget, error) {
	normalized, err := normalizeSingleStatement(statement)
	if err != nil {
		return editableSelectTarget{}, err
	}
	tokens, err := tokenizeEditableSelect(normalized)
	if err != nil {
		return editableSelectTarget{}, err
	}
	if len(tokens) < 4 || tokens[0].Upper != "SELECT" {
		return editableSelectTarget{}, fmt.Errorf("query is not a SELECT")
	}
	from := -1
	for i := 1; i < len(tokens); i++ {
		if tokens[i].Depth == 0 && tokens[i].Upper == "FROM" {
			from = i
			break
		}
	}
	if from < 2 {
		return editableSelectTarget{}, fmt.Errorf("SELECT has no direct FROM table")
	}
	for i := 1; i < from; i++ {
		if tokens[i].Depth != 0 {
			return editableSelectTarget{}, fmt.Errorf("SELECT expressions are not editable")
		}
		if tokens[i].Upper == "DISTINCT" || tokens[i].Upper == "DISTINCTROW" {
			return editableSelectTarget{}, fmt.Errorf("DISTINCT SELECT is not editable")
		}
	}
	target, next, err := parseEditableFromTarget(tokens, from+1)
	if err != nil {
		return editableSelectTarget{}, err
	}
	for i := next; i < len(tokens); i++ {
		if tokens[i].Depth != 0 {
			continue
		}
		if tokens[i].Text == "," {
			return editableSelectTarget{}, fmt.Errorf("multi-table SELECT is not editable")
		}
		switch tokens[i].Upper {
		case "JOIN", "STRAIGHT_JOIN", "LEFT", "RIGHT", "INNER", "OUTER", "CROSS", "NATURAL",
			"UNION", "INTERSECT", "EXCEPT", "GROUP", "HAVING", "INTO", "FOR", "LOCK", "PROCEDURE":
			return editableSelectTarget{}, fmt.Errorf("%s SELECT is not editable", tokens[i].Upper)
		}
	}
	selectAll, columns, err := parseEditableSelectList(tokens[1:from], target)
	if err != nil {
		return editableSelectTarget{}, err
	}
	target.SelectAll = selectAll
	target.Columns = columns
	return target, nil
}

func parseEditableFromTarget(tokens []sqlToken, start int) (editableSelectTarget, int, error) {
	if start >= len(tokens) || tokens[start].Depth != 0 || tokens[start].Kind != "ident" {
		return editableSelectTarget{}, start, fmt.Errorf("FROM must reference one table directly")
	}
	first := tokens[start].Text
	start++
	target := editableSelectTarget{Table: first}
	if start+1 < len(tokens) && tokens[start].Depth == 0 && tokens[start].Text == "." && tokens[start+1].Kind == "ident" {
		target.Catalog = first
		target.Table = tokens[start+1].Text
		start += 2
	}
	if start < len(tokens) && tokens[start].Depth == 0 && tokens[start].Upper == "AS" {
		if start+1 >= len(tokens) || tokens[start+1].Kind != "ident" {
			return editableSelectTarget{}, start, fmt.Errorf("table alias is incomplete")
		}
		target.Alias = tokens[start+1].Text
		start += 2
	} else if start < len(tokens) && tokens[start].Depth == 0 && tokens[start].Kind == "ident" && !editableClauseKeyword(tokens[start].Upper) {
		target.Alias = tokens[start].Text
		start++
	}
	return target, start, nil
}

func editableClauseKeyword(keyword string) bool {
	switch keyword {
	case "WHERE", "ORDER", "LIMIT", "OFFSET", "PROCEDURE", "INTO", "FOR", "LOCK",
		"JOIN", "STRAIGHT_JOIN", "LEFT", "RIGHT", "INNER", "OUTER", "CROSS", "NATURAL",
		"UNION", "INTERSECT", "EXCEPT", "GROUP", "HAVING":
		return true
	default:
		return false
	}
}

func parseEditableSelectList(tokens []sqlToken, target editableSelectTarget) (bool, []string, error) {
	if len(tokens) == 1 && tokens[0].Text == "*" {
		return true, nil, nil
	}
	parts := make([][]sqlToken, 0, 8)
	start := 0
	for i, token := range tokens {
		if token.Depth != 0 {
			return false, nil, fmt.Errorf("SELECT expressions are not editable")
		}
		if token.Text == "," {
			if i == start {
				return false, nil, fmt.Errorf("SELECT contains an empty column")
			}
			parts = append(parts, tokens[start:i])
			start = i + 1
		}
	}
	if start >= len(tokens) {
		return false, nil, fmt.Errorf("SELECT contains an empty column")
	}
	parts = append(parts, tokens[start:])
	columns := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		if len(part) == 1 && part[0].Text == "*" {
			return false, nil, fmt.Errorf("mixed wildcard SELECT is not editable")
		}
		var name string
		switch {
		case len(part) == 1 && part[0].Kind == "ident":
			name = part[0].Text
		case len(part) == 3 && part[0].Kind == "ident" && part[1].Text == "." && part[2].Kind == "ident":
			qualifier := part[0].Text
			if !strings.EqualFold(qualifier, target.Table) && (target.Alias == "" || !strings.EqualFold(qualifier, target.Alias)) {
				return false, nil, fmt.Errorf("SELECT column qualifier %q does not match the target table", qualifier)
			}
			name = part[2].Text
		case len(part) == 3 && part[0].Kind == "ident" && part[1].Text == "." && part[2].Text == "*":
			qualifier := part[0].Text
			if !strings.EqualFold(qualifier, target.Table) && (target.Alias == "" || !strings.EqualFold(qualifier, target.Alias)) {
				return false, nil, fmt.Errorf("SELECT wildcard qualifier %q does not match the target table", qualifier)
			}
			if len(parts) != 1 {
				return false, nil, fmt.Errorf("mixed wildcard SELECT is not editable")
			}
			return true, nil, nil
		default:
			return false, nil, fmt.Errorf("SELECT list must contain only direct table columns")
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			return false, nil, fmt.Errorf("SELECT contains duplicate column %q", name)
		}
		seen[key] = struct{}{}
		columns = append(columns, name)
	}
	return false, columns, nil
}

func tokenizeEditableSelect(statement string) ([]sqlToken, error) {
	runes := []rune(statement)
	tokens := make([]sqlToken, 0, len(runes)/3)
	depth := 0
	for i := 0; i < len(runes); {
		r := runes[i]
		if unicode.IsSpace(r) {
			i++
			continue
		}
		if r == '#' {
			for i < len(runes) && runes[i] != '\n' && runes[i] != '\r' {
				i++
			}
			continue
		}
		if r == '-' && i+1 < len(runes) && runes[i+1] == '-' && (i+2 >= len(runes) || unicode.IsSpace(runes[i+2])) {
			i += 2
			for i < len(runes) && runes[i] != '\n' && runes[i] != '\r' {
				i++
			}
			continue
		}
		if r == '/' && i+1 < len(runes) && runes[i+1] == '*' {
			end := -1
			for j := i + 2; j+1 < len(runes); j++ {
				if runes[j] == '*' && runes[j+1] == '/' {
					end = j + 2
					break
				}
			}
			if end < 0 {
				return nil, fmt.Errorf("unterminated SQL comment")
			}
			i = end
			continue
		}
		if r == '\'' || r == '"' {
			quote := r
			i++
			closed := false
			escaped := false
			for i < len(runes) {
				if escaped {
					escaped = false
					i++
					continue
				}
				if runes[i] == '\\' {
					escaped = true
					i++
					continue
				}
				if runes[i] == quote {
					if i+1 < len(runes) && runes[i+1] == quote {
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated SQL string")
			}
			tokens = append(tokens, sqlToken{Text: "<string>", Upper: "<STRING>", Kind: "string", Depth: depth})
			continue
		}
		if r == rune(96) {
			i++
			var builder strings.Builder
			closed := false
			for i < len(runes) {
				if runes[i] == rune(96) {
					if i+1 < len(runes) && runes[i+1] == rune(96) {
						builder.WriteRune(rune(96))
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				builder.WriteRune(runes[i])
				i++
			}
			if !closed || builder.Len() == 0 {
				return nil, fmt.Errorf("invalid quoted identifier")
			}
			text := builder.String()
			tokens = append(tokens, sqlToken{Text: text, Upper: strings.ToUpper(text), Kind: "ident", Depth: depth})
			continue
		}
		if r == '(' {
			tokens = append(tokens, sqlToken{Text: "(", Upper: "(", Kind: "symbol", Depth: depth})
			depth++
			i++
			continue
		}
		if r == ')' {
			if depth == 0 {
				return nil, fmt.Errorf("unbalanced SQL parenthesis")
			}
			depth--
			tokens = append(tokens, sqlToken{Text: ")", Upper: ")", Kind: "symbol", Depth: depth})
			i++
			continue
		}
		if r == ',' || r == '.' || r == '*' {
			text := string(r)
			tokens = append(tokens, sqlToken{Text: text, Upper: text, Kind: "symbol", Depth: depth})
			i++
			continue
		}
		if isEditableIdentifierRune(r, true) {
			start := i
			i++
			for i < len(runes) && isEditableIdentifierRune(runes[i], false) {
				i++
			}
			text := string(runes[start:i])
			tokens = append(tokens, sqlToken{Text: text, Upper: strings.ToUpper(text), Kind: "ident", Depth: depth})
			continue
		}
		tokens = append(tokens, sqlToken{Text: string(r), Upper: strings.ToUpper(string(r)), Kind: "symbol", Depth: depth})
		i++
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced SQL parenthesis")
	}
	return tokens, nil
}

func isEditableIdentifierRune(r rune, first bool) bool {
	if unicode.IsLetter(r) || r == '_' || r == '$' {
		return true
	}
	return !first && unicode.IsDigit(r)
}
