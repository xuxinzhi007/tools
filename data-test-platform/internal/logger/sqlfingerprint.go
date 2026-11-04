package logger

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxSQL      = 64 * 1024
	maxTemplate = 16 * 1024
)

var (
	dollarPattern = regexp.MustCompile(`^\$(?:[A-Za-z_][A-Za-z_0-9]*)?\$`)
	numberPattern = regexp.MustCompile(`^(?:0[xX][0-9a-fA-F]+|0[bB][01]+|(?:\p{Nd}+(?:\.\p{Nd}*)?|\.\p{Nd}+)(?:[eE][+-]?\p{Nd}+)?)`)
)

type tokenKind int

const (
	tokenSymbol tokenKind = iota
	tokenIdentifier
	tokenLiteral
	tokenSpace
	tokenComment
)

type sqlToken struct {
	kind   tokenKind
	text   string
	closed bool
	start  int
}

func sqlSpace(r rune) bool {
	return unicode.IsSpace(r) || r >= '\x1c' && r <= '\x1f'
}

func identifierRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$' || r == '\u00b7'
}

func quotedEnd(sql string, start int) (int, bool) {
	if start >= len(sql) {
		return len(sql), false
	}
	open := sql[start]
	close := open
	if open == '[' {
		close = ']'
	}
	for i := start + 1; i < len(sql); i++ {
		switch sql[i] {
		case '\\':
			if open == '\'' || open == '"' {
				i++
			}
		case close:
			if i+1 < len(sql) && (open == '\'' || open == '"') && sql[i+1] == close {
				i++
				continue
			}
			return i + 1, true
		}
	}
	return len(sql), false
}

// sqlTokens streams SQL into tokens without allocating the whole slice.
func sqlTokens(sql string, yield func(sqlToken) bool) {
	for i := 0; i < len(sql); {
		start := i
		r, size := utf8.DecodeRuneInString(sql[i:])
		next, _ := utf8.DecodeRuneInString(sql[i+size:])
		kind, closed := tokenSymbol, true
		switch {
		case sqlSpace(r):
			kind = tokenSpace
			for i < len(sql) {
				c, n := utf8.DecodeRuneInString(sql[i:])
				if !sqlSpace(c) {
					break
				}
				i += n
			}
		case strings.HasPrefix(sql[i:], "--") || r == '#':
			kind = tokenComment
			end := strings.IndexByte(sql[i:], '\n')
			if end < 0 {
				i = len(sql)
			} else {
				i += end
			}
		case strings.HasPrefix(sql[i:], "/*"):
			kind = tokenComment
			i += 2
			depth := 1
			for i < len(sql) && depth != 0 {
				switch {
				case strings.HasPrefix(sql[i:], "/*"):
					depth++
					i += 2
				case strings.HasPrefix(sql[i:], "*/"):
					depth--
					i += 2
				default:
					i++
				}
			}
			closed = depth == 0
		case r == '\'' || r == '"' || strings.ContainsRune("eEnNxXbB", r) && i+1 < len(sql) && (sql[i+1] == '\'' || sql[i+1] == '"'):
			kind = tokenLiteral
			quoteStart := i
			if r != '\'' && r != '"' {
				quoteStart++
			}
			i, closed = quotedEnd(sql, quoteStart)
		case r == '`' || r == '[':
			kind = tokenIdentifier
			i, closed = quotedEnd(sql, i)
		case r == '$' && dollarPattern.MatchString(sql[i:]):
			kind = tokenLiteral
			delimiter := dollarPattern.FindString(sql[i:])
			end := strings.Index(sql[i+len(delimiter):], delimiter)
			if end < 0 {
				i, closed = len(sql), false
			} else {
				i += len(delimiter)*2 + end
			}
		case unicode.IsLetter(r) || r == '_' || r == '$':
			kind = tokenIdentifier
			i += size
			for i < len(sql) {
				c, n := utf8.DecodeRuneInString(sql[i:])
				if !identifierRune(c) {
					break
				}
				i += n
			}
		case unicode.IsDigit(r) || r == '.' && unicode.IsDigit(next):
			kind = tokenLiteral
			match := numberPattern.FindString(sql[i:])
			if match == "" {
				i += size
			} else {
				i += len(match)
				if i < len(sql) {
					c, _ := utf8.DecodeRuneInString(sql[i:])
					if unicode.IsLetter(c) || c == '_' || c == '$' {
						kind = tokenIdentifier
						for i < len(sql) {
							c, n := utf8.DecodeRuneInString(sql[i:])
							if !identifierRune(c) {
								break
							}
							i += n
						}
					}
				}
			}
		default:
			i += size
		}
		text := sql[start:i]
		if kind == tokenSpace {
			text = " "
		}
		if !yield(sqlToken{kind: kind, text: text, closed: closed, start: start}) {
			return
		}
	}
}

// RedactSQL replaces every literal and unterminated fragment with "?", strips
// comments, and returns the template plus a stable fingerprint (16 hex chars).
// The bool reports whether the template was truncated or ended mid-statement,
// meaning the caller cannot trust the trailing shape.
func RedactSQL(sql string) (template, fingerprint string, truncated bool) {
	var pieces strings.Builder
	truncated = strings.HasSuffix(strings.TrimRightFunc(sql, sqlSpace), "...")
	depth, last := 0, ""
	sqlTokens(sql, func(token sqlToken) bool {
		truncated = truncated || !token.closed
		if token.kind == tokenSymbol {
			if token.text == "(" {
				depth++
			}
			if token.text == ")" {
				depth--
				truncated = truncated || depth < 0
			}
		}
		if token.kind != tokenSpace && token.kind != tokenComment && token.text != ";" {
			last = token.text
			if token.kind == tokenIdentifier {
				last = strings.ToUpper(last)
			}
			if token.kind == tokenLiteral {
				last = "?"
			}
		}
		switch {
		case token.kind == tokenLiteral || !token.closed:
			pieces.WriteByte('?')
		case token.kind == tokenComment:
			pieces.WriteString("/* redacted */")
		default:
			pieces.WriteString(token.text)
		}
		return true
	})
	switch last {
	case "SELECT", "FROM", "WHERE", "JOIN", "ON", "AND", "OR", "BY", "HAVING", "INTO", "VALUES", "SET", "AS", "UNION", "WHEN", "THEN", "ELSE", "BETWEEN", "IN", "IS", "NOT", "LIKE", "LIMIT", "OFFSET", "RETURNING", "=", ",", "+", "-", "/", "<", ">", "|", "&":
		truncated = true
	}
	truncated = truncated || depth != 0
	template = strings.TrimFunc(pieces.String(), sqlSpace)
	hash := sha256.Sum256([]byte(template))
	fingerprint = hex.EncodeToString(hash[:8])
	template, limited := LimitText(template, maxTemplate)
	if limited {
		template += "..."
	}
	return template, fingerprint, truncated || limited
}

// LimitText truncates s to limit runes and reports whether it was cut.
func LimitText(s string, limit int) (string, bool) {
	count := 0
	for i := range s {
		if count == limit {
			return s[:i], true
		}
		count++
	}
	return s, false
}
