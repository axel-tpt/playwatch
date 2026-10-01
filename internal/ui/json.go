package ui

import (
	"bytes"
	"encoding/json"
	"strings"
)

// ColorJSON pretty-prints a JSON value with syntax highlighting. Invalid JSON
// is returned unchanged.
func ColorJSON(data []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return string(data)
	}
	return colorIndented(buf.String())
}

// ColorValue marshals v and highlights it like ColorJSON.
func ColorValue(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return err.Error()
	}
	return ColorJSON(data)
}

// colorIndented highlights valid, indented JSON. Strings followed by a colon
// are object keys.
func colorIndented(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '"':
			end := i + 1
			for end < len(s) && s[end] != '"' {
				if s[end] == '\\' {
					end++
				}
				end++
			}
			end = min(end+1, len(s))
			tok := s[i:end]
			if end < len(s) && s[end] == ':' {
				b.WriteString(styleJSONKey.Render(tok))
			} else {
				b.WriteString(styleJSONString.Render(tok))
			}
			i = end
		case c == '-' || (c >= '0' && c <= '9'):
			end := i
			for end < len(s) && strings.IndexByte("+-.eE0123456789", s[end]) >= 0 {
				end++
			}
			b.WriteString(styleJSONNumber.Render(s[i:end]))
			i = end
		case strings.HasPrefix(s[i:], "true"), strings.HasPrefix(s[i:], "null"):
			b.WriteString(styleJSONLit.Render(s[i : i+4]))
			i += 4
		case strings.HasPrefix(s[i:], "false"):
			b.WriteString(styleJSONLit.Render(s[i : i+5]))
			i += 5
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}
