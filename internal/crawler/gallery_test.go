package crawler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/slinet/ehdb/internal/config"
	"go.uber.org/zap"
)

func TestGalleryCrawlerGetPagesRejectsAbnormalPage(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		_, _ = w.Write([]byte("Your IP address has been temporarily banned. (The ban expires in 4 minutes and 58 seconds)"))
	}))
	defer server.Close()

	hostURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}

	crawler := &GalleryCrawler{
		client: &Client{httpClient: server.Client(), host: hostURL.Host, cookies: map[string]string{}},
		cfg:    &config.CrawlerConfig{Host: hostURL.Host},
		logger: zap.NewNop(),
	}

	_, err = crawler.GetPages("", false)
	if err == nil {
		t.Fatal("expected abnormal page error, got nil")
	}

	if !errors.Is(err, ErrAbnormalPage) {
		t.Fatalf("expected ErrAbnormalPage, got %v", err)
	}
}

func TestGalleryCrawlerGetPagesParsesValidPage(t *testing.T) {
	body := `<html><body><div class="searchnav"></div><script>var nexturl="https://e-hentai.org/?next=3865455";</script><a href="https://e-hentai.org/g/3865624/abcdef0123/" gid=3865624&amp;t=abcdef0123&foo=1 class="posted_foo">2026-03-30 12:00</a></body></html>`
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	hostURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}

	crawler := &GalleryCrawler{
		client: &Client{httpClient: server.Client(), host: hostURL.Host, cookies: map[string]string{}},
		cfg:    &config.CrawlerConfig{Host: hostURL.Host},
		logger: zap.NewNop(),
	}

	items, err := crawler.GetPages("", false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	if items[0].Gid != "3865624" || items[0].Token != "abcdef0123" || items[0].Posted != "2026-03-30 12:00" {
		t.Fatalf("unexpected parsed item: %#v", items[0])
	}
}

func TestGalleryCrawlerGetPagesEnrichesAbnormalPageWithAPIBanProbe(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api.php":
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			_, _ = w.Write([]byte("Your IP address has been temporarily banned. (The ban expires in 4 minutes and 58 seconds)"))
		default:
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			_, _ = w.Write([]byte(`<html><title>Just a moment</title><body>Checking your browser before accessing</body></html>`))
		}
	}))
	defer server.Close()

	hostURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}

	crawler := &GalleryCrawler{
		client: &Client{httpClient: server.Client(), host: hostURL.Host, cookies: map[string]string{}},
		cfg:    &config.CrawlerConfig{Host: hostURL.Host},
		logger: zap.NewNop(),
	}

	_, err = crawler.GetPages("", false)
	if err == nil {
		t.Fatal("expected abnormal page error, got nil")
	}

	if !errors.Is(err, ErrAbnormalPage) {
		t.Fatalf("expected ErrAbnormalPage, got %v", err)
	}

	message := err.Error()
	if !strings.Contains(message, "api probe") {
		t.Fatalf("expected api probe reason in error, got %q", message)
	}
	if !strings.Contains(message, "ban expires in 4 minutes and 58 seconds") {
		t.Fatalf("expected ban expiry in error, got %q", message)
	}
}
