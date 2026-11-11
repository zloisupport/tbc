package util

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"resty.dev/v3"
)

const (
	cookieEnvName = "TERABOX_COOKIE"
	// DefaultCookieFile is read when neither -c nor TERABOX_COOKIE is set.
	DefaultCookieFile = "cookie"
)

func FindFilesRecursive(rootPath string) ([]string, error) {
	var fileList []string
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("Error accessing path '%s': %v\n", path, err)
			return nil // Skip this path and continue walking
		}
		// Skip the root directory itself
		if path == rootPath {
			return nil
		}
		// Check for hidden files (starts with .) and skip
		if !info.IsDir() {
			fileList = append(fileList, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return fileList, nil
}

func GetResponse(resp *resty.Response, err error) ([]byte, error) {
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, fmt.Errorf("request failed with status: %d", resp.StatusCode())
	}

	return resp.Bytes(), nil
}

func getCookieFromEnv() (string, error) {
	cookie := cleanCookie(os.Getenv(cookieEnvName))
	if cookie == "" {
		return "", fmt.Errorf("TeraBox Cookie not found: set %s or pass -c <file>", cookieEnvName)
	}
	return cookie, nil
}

// cleanCookie removes whitespace and a UTF-8 BOM that editors such as
// Notepad may add to a cookie file. Both would end up in the cookie name
// and make the session unrecognizable to TeraBox.
func cleanCookie(s string) string {
	s = strings.TrimPrefix(s, "\uFEFF")
	return strings.TrimSpace(s)
}

func GetCookie(filePath string) (string, error) {
	if filePath != "" {
		return readCookieFile(filePath)
	}

	// Prefer the environment, then fall back to the conventional ./cookie
	// file that this project's README suggests.
	if cookie, err := getCookieFromEnv(); err == nil {
		return cookie, nil
	}
	if _, err := os.Stat(DefaultCookieFile); err == nil {
		return readCookieFile(DefaultCookieFile)
	}

	return "", fmt.Errorf("TeraBox Cookie not found: set %s, pass -c <file>, or create ./%s",
		cookieEnvName, DefaultCookieFile)
}

func readCookieFile(filePath string) (string, error) {
	fp, err := os.Open(filePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, filePath, "not found")
		return getCookieFromEnv()
	}
	defer fp.Close()

	data, err := io.ReadAll(fp)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to read cookie file")
		return getCookieFromEnv()
	}

	// A trailing newline, surrounding spaces or a UTF-8 BOM make the ndus
	// value invalid, so strip all of them.
	cookie := cleanCookie(string(data))
	if cookie == "" {
		return "", fmt.Errorf("cookie file %s is empty", filePath)
	}
	return cookie, nil
}

func GetAbsPath(cwd, relPath string) string {
	if !strings.HasPrefix(relPath, "/") {
		return path.Join(cwd, relPath)
	}
	return path.Clean(relPath)
}

func ByteCountSI(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c",
		float64(b)/float64(div), "kMGTPE"[exp])
}

func ByteCountIEC(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c",
		float64(b)/float64(div), "KMGTPE"[exp])
}

func ParseChunkSize(arg string) (int64, error) {
	if arg == "" {
		return 0, fmt.Errorf("chunk-size cannot be empty")
	}

	numStr := arg
	unit := strings.ToLower(string(numStr[len(numStr)-1]))

	if unit < "0" || unit > "9" {
		numStr = arg[:len(arg)-1]
		if numStr == "" {
			return 0, fmt.Errorf("invalid format: %s", arg)
		}
	} else {
		unit = ""
	}

	base, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number format: %s", numStr)
	}

	if base < 1 {
		return 0, fmt.Errorf("value must be greater than 0")
	}

	switch unit {
	case "k":
		base *= 1024
	case "m":
		base *= 1024 * 1024
	case "g":
		base *= 1024 * 1024 * 1024
	case "":
	default:
		return 0, fmt.Errorf("invalid unit: %s (use k/K/m/M/g/G)", unit)
	}

	base = min(1024*1024*1024*1024, base)

	return base, nil
}
