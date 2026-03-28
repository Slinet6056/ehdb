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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/slinet/ehdb/internal/config"
	"golang.org/x/net/proxy"
)

// Client is an HTTP client with proxy and cookie support
type Client struct {
	httpClient *http.Client
	host       string

	mu      sync.RWMutex
	cookies map[string]string
}

// NewClient creates a new crawler client
func NewClient(cfg *config.CrawlerConfig) (*Client, error) {
	client := &Client{
		host:    cfg.Host,
		cookies: parseCookieHeader(cfg.Cookies),
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

func parseCookieHeader(raw string) map[string]string {
	cookies := make(map[string]string)

	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		name, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}

		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" || value == "" {
			continue
		}

		cookies[name] = value
	}

	return cookies
}

func buildCookieHeader(cookies map[string]string) string {
	if len(cookies) == 0 {
		return ""
	}

	keys := make([]string, 0, len(cookies))
	for name, value := range cookies {
		if name == "" || value == "" {
			continue
		}
		keys = append(keys, name)
	}

	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+cookies[key])
	}

	return strings.Join(parts, "; ")
}

func (c *Client) cookieHeader() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return buildCookieHeader(c.cookies)
}

func (c *Client) cookieValue(name string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	value, ok := c.cookies[name]
	return value, ok
}

func (c *Client) updateCookies(resp *http.Response) {
	if resp == nil {
		return
	}

	responseCookies := resp.Cookies()
	if len(responseCookies) == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cookies == nil {
		c.cookies = make(map[string]string)
	}

	for _, cookie := range responseCookies {
		if cookie.Name == "" {
			continue
		}

		if cookie.Value == "" || cookie.MaxAge < 0 {
			delete(c.cookies, cookie.Name)
			continue
		}

		c.cookies[cookie.Name] = cookie.Value
	}
}

func (c *Client) detectExHentaiAuthFailure(resp *http.Response, body []byte) (string, bool) {
	if !strings.EqualFold(c.host, "exhentai.org") || resp == nil {
		return "", false
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "text/html") {
		return "", false
	}

	igneous, ok := c.cookieValue("igneous")
	if !ok || !strings.EqualFold(igneous, "mystery") {
		return "", false
	}

	if strings.Contains(contentType, "text/html") && len(bytes.TrimSpace(body)) == 0 {
		return "received blank HTML with igneous=mystery", true
	}

	return "", false
}

func (c *Client) validateResponse(resp *http.Response, body []byte) error {
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("auth failed with status code %d: %w", resp.StatusCode, ErrAuthRequired)
	}

	if marker, ok := isAuthFailureBody(body); ok {
		return fmt.Errorf("auth failed, detected marker %q: %w", marker, ErrAuthRequired)
	}

	if reason, ok := c.detectExHentaiAuthFailure(resp, body); ok {
		return fmt.Errorf("auth failed, detected ExHentai session issue (%s): %w", reason, ErrAuthRequired)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
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

	if cookieHeader := c.cookieHeader(); cookieHeader != "" {
		req.Header.Set("Cookie", cookieHeader)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	c.updateCookies(resp)

	if err := c.validateResponse(resp, body); err != nil {
		return nil, err
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

	if cookieHeader := c.cookieHeader(); cookieHeader != "" {
		req.Header.Set("Cookie", cookieHeader)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	c.updateCookies(resp)

	if err := c.validateResponse(resp, body); err != nil {
		return nil, err
	}

	return body, nil
}
