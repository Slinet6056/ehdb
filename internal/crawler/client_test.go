package crawler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/slinet/ehdb/internal/config"
)

func TestClientValidateResponseRejectsBlankExHentaiPageWithMysteryIgneous(t *testing.T) {
	t.Helper()

	requestURL, err := url.Parse("https://exhentai.org/")
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}

	client := &Client{
		host:    "exhentai.org",
		cookies: parseCookieHeader("ipb_member_id=1; ipb_pass_hash=hash; igneous=old"),
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/html; charset=UTF-8"},
			"Set-Cookie":   []string{"igneous=mystery; Path=/; Domain=.exhentai.org"},
		},
		Request: &http.Request{URL: requestURL},
	}

	if err := client.updateCookies(resp); err != nil {
		t.Fatalf("update cookies: %v", err)
	}
	err = client.validateResponse(resp, []byte{})
	if err == nil {
		t.Fatal("expected auth error, got nil")
	}

	if !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("expected ErrAuthRequired, got %v", err)
	}

	igneous, ok := client.cookieValue("igneous")
	if !ok || igneous != "mystery" {
		t.Fatalf("expected igneous to update to mystery, got %q", igneous)
	}
}

func TestClientValidateResponseAllowsExHentaiAPIJSONResponse(t *testing.T) {
	t.Helper()

	requestURL, err := url.Parse("https://api.e-hentai.org/api.php")
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}

	client := &Client{
		host:    "exhentai.org",
		cookies: parseCookieHeader("ipb_member_id=1; ipb_pass_hash=hash; igneous=abc"),
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json; charset=UTF-8"},
		},
		Request: &http.Request{URL: requestURL},
	}

	err = client.validateResponse(resp, []byte(`{"gmetadata":[]}`))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestClientUpdateCookiesRefreshesIgneous(t *testing.T) {
	t.Helper()

	client := &Client{
		host:    "exhentai.org",
		cookies: parseCookieHeader("ipb_member_id=1; ipb_pass_hash=hash; igneous=old"),
	}

	resp := &http.Response{
		Header: http.Header{
			"Set-Cookie": []string{"igneous=fresh; Path=/; Domain=.exhentai.org"},
		},
	}

	if err := client.updateCookies(resp); err != nil {
		t.Fatalf("update cookies: %v", err)
	}

	igneous, ok := client.cookieValue("igneous")
	if !ok {
		t.Fatal("expected igneous cookie to exist")
	}

	if igneous != "fresh" {
		t.Fatalf("expected igneous to refresh to fresh, got %q", igneous)
	}
}

func TestNewClientPrefersCookiesFileOverConfig(t *testing.T) {
	t.Helper()

	setCookiesFilePathForTest(t, filepath.Join(t.TempDir(), "cookies.json"))

	if err := persistCookiesToFile(resolveCookiesFilePath(""), map[string]string{
		"igneous":       "fresh",
		"ipb_member_id": "2",
	}); err != nil {
		t.Fatalf("persist cookies file: %v", err)
	}

	client, err := NewClient(&config.CrawlerConfig{
		Host:      "exhentai.org",
		Cookies:   "igneous=stale; ipb_member_id=1; ipb_pass_hash=hash",
		ConfigDir: filepath.Dir(cookiesFilePath),
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	igneous, ok := client.cookieValue("igneous")
	if !ok || igneous != "fresh" {
		t.Fatalf("expected cookies file to override igneous, got %q", igneous)
	}

	ipbMemberID, ok := client.cookieValue("ipb_member_id")
	if !ok || ipbMemberID != "2" {
		t.Fatalf("expected cookies file to override ipb_member_id, got %q", ipbMemberID)
	}

	ipbPassHash, ok := client.cookieValue("ipb_pass_hash")
	if !ok || ipbPassHash != "hash" {
		t.Fatalf("expected config cookie to remain available, got %q", ipbPassHash)
	}
}

func TestClientUpdateCookiesPersistsToFile(t *testing.T) {
	t.Helper()

	setCookiesFilePathForTest(t, filepath.Join(t.TempDir(), "cookies.json"))

	client := &Client{
		host:        "exhentai.org",
		cookiesPath: cookiesFilePath,
		cookies:     parseCookieHeader("ipb_member_id=1; ipb_pass_hash=hash; igneous=old"),
	}

	resp := &http.Response{
		Header: http.Header{
			"Set-Cookie": []string{"igneous=fresh; Path=/; Domain=.exhentai.org"},
		},
	}

	if err := client.updateCookies(resp); err != nil {
		t.Fatalf("update cookies: %v", err)
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
		t.Fatalf("expected persisted igneous cookie to refresh to fresh, got %q", cookies["igneous"])
	}
	if cookies["ipb_member_id"] != "1" {
		t.Fatalf("expected persisted ipb_member_id cookie to remain, got %q", cookies["ipb_member_id"])
	}
	if cookies["ipb_pass_hash"] != "hash" {
		t.Fatalf("expected persisted ipb_pass_hash cookie to remain, got %q", cookies["ipb_pass_hash"])
	}
}

func TestClientUpdateCookiesRemovesExpiredCookie(t *testing.T) {
	t.Helper()

	setCookiesFilePathForTest(t, filepath.Join(t.TempDir(), "cookies.json"))

	client := &Client{
		host:        "exhentai.org",
		cookiesPath: cookiesFilePath,
		cookies:     parseCookieHeader("igneous=fresh; ipb_member_id=1"),
	}

	resp := &http.Response{
		Header: http.Header{
			"Set-Cookie": []string{"igneous=expired; Expires=Thu, 01 Jan 1970 00:00:00 GMT; Path=/; Domain=.exhentai.org"},
		},
	}

	if err := client.updateCookies(resp); err != nil {
		t.Fatalf("update cookies: %v", err)
	}

	if _, ok := client.cookieValue("igneous"); ok {
		t.Fatal("expected expired igneous cookie to be removed")
	}
}

func TestClientUpdateCookiesBootstrapsCookiesFileWhenMissing(t *testing.T) {
	t.Helper()

	setCookiesFilePathForTest(t, filepath.Join(t.TempDir(), "cookies.json"))

	client := &Client{
		host:        "exhentai.org",
		cookiesPath: cookiesFilePath,
		cookies:     parseCookieHeader("igneous=fresh; ipb_member_id=1; ipb_pass_hash=hash"),
	}

	resp := &http.Response{Header: http.Header{}}

	if err := client.updateCookies(resp); err != nil {
		t.Fatalf("update cookies: %v", err)
	}

	data, err := os.ReadFile(cookiesFilePath)
	if err != nil {
		t.Fatalf("read cookies file: %v", err)
	}

	var cookies map[string]string
	if err := json.Unmarshal(data, &cookies); err != nil {
		t.Fatalf("unmarshal cookies file: %v", err)
	}

	if cookies["igneous"] != "fresh" || cookies["ipb_member_id"] != "1" || cookies["ipb_pass_hash"] != "hash" {
		t.Fatalf("unexpected bootstrapped cookies: %#v", cookies)
	}
}

func TestClientUpdateCookiesDoesNotRewriteExistingCookiesFileWithoutChanges(t *testing.T) {
	t.Helper()

	setCookiesFilePathForTest(t, filepath.Join(t.TempDir(), "cookies.json"))

	if err := os.WriteFile(cookiesFilePath, []byte("{\n  \"igneous\": \"existing\"\n}\n"), 0o600); err != nil {
		t.Fatalf("write cookies file: %v", err)
	}

	client := &Client{
		host:        "exhentai.org",
		cookiesPath: cookiesFilePath,
		cookies:     parseCookieHeader("igneous=fresh; ipb_member_id=1; ipb_pass_hash=hash"),
	}

	resp := &http.Response{Header: http.Header{}}

	if err := client.updateCookies(resp); err != nil {
		t.Fatalf("update cookies: %v", err)
	}

	data, err := os.ReadFile(cookiesFilePath)
	if err != nil {
		t.Fatalf("read cookies file: %v", err)
	}

	if string(data) != "{\n  \"igneous\": \"existing\"\n}\n" {
		t.Fatalf("expected existing cookies file to remain unchanged, got %q", string(data))
	}
}

func TestClientValidateResponseRejectsExistingMysteryIgneousOnBlankHTML(t *testing.T) {
	t.Helper()

	requestURL, err := url.Parse("https://exhentai.org/")
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}

	client := &Client{
		host:    "exhentai.org",
		cookies: parseCookieHeader("ipb_member_id=1; ipb_pass_hash=hash; igneous=mystery"),
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/html; charset=UTF-8"},
		},
		Request: &http.Request{URL: requestURL},
	}

	err = client.validateResponse(resp, []byte("   \n\t"))
	if err == nil {
		t.Fatal("expected auth error, got nil")
	}

	if !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("expected ErrAuthRequired, got %v", err)
	}
}

func setCookiesFilePathForTest(t *testing.T, path string) {
	t.Helper()

	originalPath := cookiesFilePath
	cookiesFilePath = path
	t.Cleanup(func() {
		cookiesFilePath = originalPath
	})
}
