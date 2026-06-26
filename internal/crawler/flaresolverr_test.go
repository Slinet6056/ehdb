package crawler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFlareSolverrGetRejectsEmptyExHentaiShellAndDoesNotPersistMysteryCookie(t *testing.T) {
	t.Helper()

	setCookiesFilePathForTest(t, filepath.Join(t.TempDir(), "cookies.json"))
	if err := persistCookiesToFile(cookiesFilePath, map[string]string{
		"igneous":       "fresh",
		"ipb_member_id": "1",
		"ipb_pass_hash": "hash",
	}); err != nil {
		t.Fatalf("persist initial cookies: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := flareSolverrResponse{
			Status:  "ok",
			Message: "Challenge not detected!",
			Solution: flareSolverrSolution{
				URL:      "https://exhentai.org/",
				Status:   http.StatusOK,
				Headers:  map[string]string{"Content-Type": "text/html; charset=UTF-8"},
				Response: "<html><head></head><body></body></html>",
				Cookies: []flareSolverrCookie{
					{Name: "igneous", Value: "mystery"},
					{Name: "ipb_member_id", Value: "1"},
					{Name: "ipb_pass_hash", Value: "hash"},
				},
			},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode flaresolverr response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		host:        "exhentai.org",
		cookiesPath: cookiesFilePath,
		cookies:     parseCookieHeader("igneous=fresh; ipb_member_id=1; ipb_pass_hash=hash"),
	}

	_, err := client.flareSolverrGet("https://exhentai.org/", server.URL)
	if err == nil {
		t.Fatal("expected auth error, got nil")
	}
	if !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("expected ErrAuthRequired, got %v", err)
	}

	data, err := os.ReadFile(cookiesFilePath)
	if err != nil {
		t.Fatalf("read cookies file: %v", err)
	}

	var cookies map[string]string
	if err := json.Unmarshal(data, &cookies); err != nil {
		t.Fatalf("unmarshal cookies file: %v", err)
	}
	if cookies["igneous"] != "fresh" {
		t.Fatalf("expected persisted igneous to remain fresh, got %q", cookies["igneous"])
	}
}

func TestFlareSolverrGetPersistsCookiesAfterValidResponse(t *testing.T) {
	t.Helper()

	setCookiesFilePathForTest(t, filepath.Join(t.TempDir(), "cookies.json"))
	if err := persistCookiesToFile(cookiesFilePath, map[string]string{
		"igneous":       "old",
		"ipb_member_id": "1",
		"ipb_pass_hash": "hash",
	}); err != nil {
		t.Fatalf("persist initial cookies: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := flareSolverrResponse{
			Status:  "ok",
			Message: "Challenge not detected!",
			Solution: flareSolverrSolution{
				URL:      "https://exhentai.org/",
				Status:   http.StatusOK,
				Headers:  map[string]string{"Content-Type": "text/html; charset=UTF-8"},
				Response: `<html><body><div class="searchnav"></div><table class="itg"></table><a href="?next=123">nexturl=123</a><div id="posted_1"></div></body></html>`,
				Cookies: []flareSolverrCookie{
					{Name: "igneous", Value: "fresh"},
					{Name: "ipb_member_id", Value: "1"},
					{Name: "ipb_pass_hash", Value: "hash"},
				},
			},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode flaresolverr response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		host:        "exhentai.org",
		cookiesPath: cookiesFilePath,
		cookies:     parseCookieHeader("igneous=old; ipb_member_id=1; ipb_pass_hash=hash"),
	}

	body, err := client.flareSolverrGet("https://exhentai.org/", server.URL)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(body) == 0 {
		t.Fatal("expected response body")
	}

	igneous, ok := client.cookieValue("igneous")
	if !ok || igneous != "fresh" {
		t.Fatalf("expected in-memory igneous to refresh to fresh, got %q", igneous)
	}

	data, err := os.ReadFile(cookiesFilePath)
	if err != nil {
		t.Fatalf("read cookies file: %v", err)
	}

	var cookies map[string]string
	if err := json.Unmarshal(data, &cookies); err != nil {
		t.Fatalf("unmarshal cookies file: %v", err)
	}
	if cookies["igneous"] != "fresh" {
		t.Fatalf("expected persisted igneous to refresh to fresh, got %q", cookies["igneous"])
	}
}
