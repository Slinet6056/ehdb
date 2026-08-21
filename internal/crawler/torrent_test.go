package crawler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

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

func TestTorrentProcessingFailureStopsSyncOnRetryExhaustion(t *testing.T) {
	crawler := &TorrentCrawler{logger: zap.NewNop()}

	err := crawler.torrentProcessingFailure(123456, fmt.Errorf("exceeded max retries (3): %w", ErrAbnormalPage))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrAbnormalPage) {
		t.Fatalf("expected ErrAbnormalPage, got %v", err)
	}

	message := err.Error()
	if !strings.Contains(message, "failed to process gallery 123456 torrents") {
		t.Fatalf("expected stop-sync error message, got %q", message)
	}
	if !strings.Contains(message, "exceeded max retries (3)") {
		t.Fatalf("expected retry exhaustion in error message, got %q", message)
	}
}

func TestOrderedGalleryIDsByMaxGTID(t *testing.T) {
	gidMap := map[int][]TorrentListItem{
		30: {
			{Gid: 30, Gtid: 330},
			{Gid: 30, Gtid: 310},
		},
		10: {
			{Gid: 10, Gtid: 120},
			{Gid: 10, Gtid: 110},
		},
		20: {
			{Gid: 20, Gtid: 220},
		},
		40: {
			{Gid: 40, Gtid: 220},
		},
	}

	got := orderedGalleryIDsByMaxGTID(gidMap)
	want := []int{10, 20, 40, 30}

	if len(got) != len(want) {
		t.Fatalf("unexpected ordered gid count: got %d want %d", len(got), len(want))
	}

	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("unexpected gid order at %d: got %v want %v", index, got, want)
		}
	}
}

func TestTorrentCrawlerParseTorrentsSanitizesInvalidUTF8(t *testing.T) {
	html := append([]byte(`name="gtid" value="2188462" Posted:<x>2026-08-21 10:00</x> Size:>1.0 MiB</x> Uploader:user`), 0xe3)
	html = append(html, []byte(`</x> 0123456789abcdef0123456789abcdef01234567.torrent">bad`)...)
	html = append(html, 0xe3)
	html = append(html, []byte(`name</a></td>`)...)

	crawler := &TorrentCrawler{}
	torrents := crawler.parseTorrents(html, 4134857)
	if len(torrents) != 1 {
		t.Fatalf("expected one torrent, got %d", len(torrents))
	}

	torrent := torrents[0]
	if !utf8.ValidString(torrent.Name) || torrent.Name != "bad\uFFFDname" {
		t.Fatalf("expected sanitized torrent name, got %q", torrent.Name)
	}
	if !utf8.ValidString(torrent.Uploader) || torrent.Uploader != "user\uFFFD" {
		t.Fatalf("expected sanitized uploader, got %q", torrent.Uploader)
	}
}
