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

func TestTorrentCrawlerFetchTorrentListPageEnrichesAbnormalPageWithAPIBanProbe(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api.php":
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			_, _ = w.Write([]byte("Your IP address has been temporarily banned. (The ban expires in 9 minutes and 12 seconds)"))
		case "/torrents.php":
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			_, _ = w.Write([]byte(`<html><title>Just a moment</title><body>Checking your browser before accessing</body></html>`))
		default:
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			_, _ = w.Write([]byte("unexpected"))
		}
	}))
	defer server.Close()

	hostURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}

	crawler := &TorrentCrawler{
		client: &Client{httpClient: server.Client(), host: hostURL.Host, cookies: map[string]string{}},
		cfg:    &config.CrawlerConfig{Host: hostURL.Host},
		logger: zap.NewNop(),
	}

	_, err = crawler.fetchTorrentListPage(0)
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
	if !strings.Contains(message, "ban expires in 9 minutes and 12 seconds") {
		t.Fatalf("expected ban expiry in error, got %q", message)
	}
}
