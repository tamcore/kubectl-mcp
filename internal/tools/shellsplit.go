package tools

import "fmt"

// shellSplit splits a command string into tokens, respecting single and double
// quotes so that e.g. `sh -c "echo hello"` becomes ["sh", "-c", "echo hello"].
// Backslash escapes are honoured inside double-quoted strings and at the top
// level, but not inside single-quoted strings (matching POSIX sh behaviour).
func shellSplit(s string) ([]string, error) {
	var (
		tokens []string
		cur    []byte
		inSQ   bool // inside single quotes
		inDQ   bool // inside double quotes
	)

	for i := 0; i < len(s); i++ {
		c := s[i]

		switch {
		case inSQ:
			if c == '\'' {
				inSQ = false
			} else {
				cur = append(cur, c)
			}

		case inDQ:
			if c == '\\' && i+1 < len(s) {
				i++
				cur = append(cur, s[i])
			} else if c == '"' {
				inDQ = false
			} else {
				cur = append(cur, c)
			}

		case c == '\\' && i+1 < len(s):
			i++
			cur = append(cur, s[i])

		case c == '\'':
			inSQ = true

		case c == '"':
			inDQ = true

		case c == ' ' || c == '\t':
			if len(cur) > 0 {
				tokens = append(tokens, string(cur))
				cur = cur[:0]
			}

		default:
			cur = append(cur, c)
		}
	}

	if inSQ {
		return nil, fmt.Errorf("unterminated single quote")
	}
	if inDQ {
		return nil, fmt.Errorf("unterminated double quote")
	}

	if len(cur) > 0 {
		tokens = append(tokens, string(cur))
	}
	return tokens, nil
}
