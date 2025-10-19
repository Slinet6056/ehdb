package crawler

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/slinet/ehdb/internal/config"
	"golang.org/x/net/proxy"
)

// Client is an HTTP client with proxy and cookie support
type Client struct {
	httpClient *http.Client
	cookies    string
	host       string
}

// NewClient creates a new crawler client
func NewClient(cfg *config.CrawlerConfig) (*Client, error) {
	client := &Client{
		cookies: cfg.Cookies,
		host:    cfg.Host,
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	// Setup proxy if configured
	if cfg.Proxy != "" {
		proxyURL, err := url.Parse(cfg.Proxy)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}

		if proxyURL.Scheme == "socks5" {
			// SOCKS5 proxy
			dialer, err := proxy.SOCKS5("tcp", proxyURL.Host, nil, proxy.Direct)
			if err != nil {
				return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
			}
			transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				return dialer.Dial(network, address)
			}
		} else {
			// HTTP/HTTPS proxy
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	client.httpClient = &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	return client, nil
}

// Get performs a GET request
func (c *Client) Get(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set headers
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*")
	req.Header.Set("Accept-Language", "en-US;q=0.9,en;q=0.8")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", fmt.Sprintf("https://%s", c.host))
	req.Header.Set("DNT", "1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	if c.cookies != "" {
		req.Header.Set("Cookie", c.cookies)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return body, nil
}

// Post performs a POST request with JSON body
func (c *Client) Post(url string, jsonData []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set headers
	req.Header.Set("Accept", "application/json;q=0.9,*/*")
	req.Header.Set("Accept-Language", "en-US;q=0.9,en;q=0.8")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("DNT", "1")
	req.ContentLength = int64(len(jsonData))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return body, nil
}
