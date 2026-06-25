package config

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ParseEnvFile reads KEY=VALUE pairs from r and returns them as a map. It
// treats the input as plain data: contents are never sourced, executed, or
// shell-expanded (closes upstream SEC3). Blank lines and lines beginning with
// '#' are ignored; a value may be wrapped in one layer of matching single or
// double quotes. A line without '=' or with an empty key is a clear error.
func ParseEnvFile(r io.Reader) (map[string]string, error) {
	out := make(map[string]string)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rawKey, value, found := strings.Cut(line, "=")
		if !found {
			return nil, fmt.Errorf("malformed env-file line (no '='): %q", line)
		}
		key := strings.TrimSpace(rawKey)
		if key == "" {
			return nil, fmt.Errorf("malformed env-file line (empty key): %q", line)
		}
		out[key] = unquote(strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading env-file: %w", err)
	}
	return out, nil
}

// unquote strips one layer of matching single or double quotes. Quotes protect
// interior whitespace; an unbalanced or mismatched quote is left literal.
func unquote(v string) string {
	if len(v) >= 2 {
		if c := v[0]; (c == '"' || c == '\'') && v[len(v)-1] == c {
			return v[1 : len(v)-1]
		}
	}
	return v
}
