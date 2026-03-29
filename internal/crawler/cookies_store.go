package crawler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

var cookiesFilePath = "cookies.json"

func resolveCookiesFilePath(configDir string) string {
	if filepath.IsAbs(cookiesFilePath) {
		return cookiesFilePath
	}

	if configDir == "" || configDir == "." {
		return cookiesFilePath
	}

	return filepath.Join(configDir, filepath.Base(cookiesFilePath))
}

func loadCookiesFromFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("read cookies file: %w", err)
	}

	var cookies map[string]string
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil, fmt.Errorf("unmarshal cookies file: %w", err)
	}

	return normalizeCookies(cookies), nil
}

func cookiesFileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}

	return false, fmt.Errorf("stat cookies file: %w", err)
}

func persistCookiesToFile(path string, cookies map[string]string) error {
	normalized := normalizeCookies(cookies)
	if len(normalized) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove cookies file: %w", err)
		}

		return nil
	}

	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}

	tempFile, err := os.CreateTemp(dir, ".cookies.json.*")
	if err != nil {
		return fmt.Errorf("create temp cookies file: %w", err)
	}

	tempPath := tempFile.Name()
	cleanupTempFile := func() {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
	}

	if err := tempFile.Chmod(0o600); err != nil {
		cleanupTempFile()
		return fmt.Errorf("chmod temp cookies file: %w", err)
	}

	encoder := json.NewEncoder(tempFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(normalized); err != nil {
		cleanupTempFile()
		return fmt.Errorf("encode cookies file: %w", err)
	}

	if err := tempFile.Sync(); err != nil {
		cleanupTempFile()
		return fmt.Errorf("sync cookies file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close temp cookies file: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace cookies file: %w", err)
	}

	return nil
}

func normalizeCookies(cookies map[string]string) map[string]string {
	if len(cookies) == 0 {
		return nil
	}

	normalized := make(map[string]string, len(cookies))
	for name, value := range cookies {
		if name == "" || value == "" {
			continue
		}

		normalized[name] = value
	}

	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

func mergeCookies(base, override map[string]string) map[string]string {
	merged := normalizeCookies(base)
	for name, value := range normalizeCookies(override) {
		if merged == nil {
			merged = make(map[string]string)
		}

		merged[name] = value
	}

	return merged
}
