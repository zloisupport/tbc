package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetCookieTrimsBOMAndWhitespace(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "cookie")
	// Notepad saves UTF-8 cookies with a BOM and CRLF line ending.
	if err := os.WriteFile(file, []byte("\uFEFFndus=abc123\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cookie, err := GetCookie(file)
	if err != nil {
		t.Fatalf("GetCookie failed: %v", err)
	}
	if cookie != "ndus=abc123" {
		t.Fatalf("expected %q, got %q", "ndus=abc123", cookie)
	}
}

func TestGetCookieFromEnv(t *testing.T) {
	t.Setenv(cookieEnvName, "  ndus=env123  ")

	cookie, err := GetCookie("")
	if err != nil {
		t.Fatalf("GetCookie failed: %v", err)
	}
	if cookie != "ndus=env123" {
		t.Fatalf("expected %q, got %q", "ndus=env123", cookie)
	}
}
