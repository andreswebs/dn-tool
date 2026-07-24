package config

import (
	"strings"
	"testing"
)

func TestParseEnvFile_MalformedLineErrors(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantText string
	}{
		{"no equals sign", "DN_VALID=ok\nGARBAGE LINE\n", "GARBAGE LINE"},
		{"empty key", "=value\n", "=value"},
		{"whitespace-only key", "   =value\n", "=value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEnvFile(strings.NewReader(tt.input))
			if err == nil {
				t.Fatalf("ParseEnvFile() error = nil, want error for %q", tt.input)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("ParseEnvFile() error = %q, want it to name %q", err, tt.wantText)
			}
			if got != nil {
				t.Errorf("ParseEnvFile() map = %v, want nil on error", got)
			}
		})
	}
}

func TestParseEnvFile_SimplePair(t *testing.T) {
	got, err := ParseEnvFile(strings.NewReader("DN_NETWORK_ID=net-123\n"))
	if err != nil {
		t.Fatalf("ParseEnvFile() error = %v", err)
	}
	if len(got) != 1 || got["DN_NETWORK_ID"] != "net-123" {
		t.Errorf("ParseEnvFile() = %v, want {DN_NETWORK_ID: net-123}", got)
	}
}

func TestParseEnvFile_CommentsAndBlankLinesIgnored(t *testing.T) {
	input := "# a comment\n\n  # indented comment\nDN_ROLE_ID=role-1\n\n"
	got, err := ParseEnvFile(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseEnvFile() error = %v", err)
	}
	if len(got) != 1 || got["DN_ROLE_ID"] != "role-1" {
		t.Errorf("ParseEnvFile() = %v, want only {DN_ROLE_ID: role-1}", got)
	}
}

func TestParseEnvFile_KeyWhitespaceTrimmed(t *testing.T) {
	got, err := ParseEnvFile(strings.NewReader("  DN_API_KEY = secret\n"))
	if err != nil {
		t.Fatalf("ParseEnvFile() error = %v", err)
	}
	if got["DN_API_KEY"] != "secret" {
		t.Errorf("ParseEnvFile()[DN_API_KEY] = %q, want %q", got["DN_API_KEY"], "secret")
	}
}

// TestParseEnvFile_ShellMetacharactersAreLiteral is the SEC3 guard: the parser
// must treat file contents as data, never sourcing, executing, or expanding
// them. Each value must survive byte-for-byte.
func TestParseEnvFile_ShellMetacharactersAreLiteral(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{"command substitution", "K=$(rm -rf /)", "$(rm -rf /)"},
		{"backtick substitution", "K=`id`", "`id`"},
		{"variable reference", "K=$OTHER", "$OTHER"},
		{"semicolon and ampersand", "K=a; reboot && id", "a; reboot && id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEnvFile(strings.NewReader(tt.line + "\n"))
			if err != nil {
				t.Fatalf("ParseEnvFile() error = %v", err)
			}
			if got["K"] != tt.want {
				t.Errorf("ParseEnvFile()[K] = %q, want literal %q", got["K"], tt.want)
			}
		})
	}
}

func TestParseEnvFile_QuotedValuesUnwrapped(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{"double quotes", `K="a b"`, "a b"},
		{"single quotes", `K='x'`, "x"},
		{"quotes preserve interior whitespace", `K="  spaced  "`, "  spaced  "},
		{"empty quoted value", `K=""`, ""},
		{"unbalanced quote kept literal", `K="oops`, `"oops`},
		{"mismatched quotes kept literal", `K="x'`, `"x'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEnvFile(strings.NewReader(tt.line + "\n"))
			if err != nil {
				t.Fatalf("ParseEnvFile() error = %v", err)
			}
			if got["K"] != tt.want {
				t.Errorf("ParseEnvFile()[K] = %q, want %q", got["K"], tt.want)
			}
		})
	}
}
