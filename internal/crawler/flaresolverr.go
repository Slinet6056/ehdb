package crawler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type flareSolverrRequest struct {
	CMD        string               `json:"cmd"`
	URL        string               `json:"url"`
	MaxTimeout int                  `json:"maxTimeout"`
	Cookies    []flareSolverrCookie `json:"cookies,omitempty"`
}

type flareSolverrCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type flareSolverrSolution struct {
	URL      string               `json:"url"`
	Status   int                  `json:"status"`
	Headers  map[string]string    `json:"headers"`
	Response string               `json:"response"`
	Cookies  []flareSolverrCookie `json:"cookies"`
}

type flareSolverrResponse struct {
	Status   string               `json:"status"`
	Message  string               `json:"message"`
	Solution flareSolverrSolution `json:"solution"`
}

// flareSolverrGet sends a GET request through FlareSolverr and returns the response body.
// It also syncs any cookies returned by FlareSolverr back into the client.
func (c *Client) flareSolverrGet(targetURL, serviceURL string) ([]byte, error) {
	c.mu.RLock()
	var cookies []flareSolverrCookie
	for name, value := range c.cookies {
		cookies = append(cookies, flareSolverrCookie{Name: name, Value: value})
	}
	c.mu.RUnlock()

	payload, err := json.Marshal(flareSolverrRequest{
		CMD:        "request.get",
		URL:        targetURL,
		MaxTimeout: 60000,
		Cookies:    cookies,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal flaresolverr request: %w", err)
	}

	req, err := http.NewRequest("POST", serviceURL+"/v1", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create flaresolverr request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 90 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("flaresolverr request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read flaresolverr response: %w", err)
	}

	var fsResp flareSolverrResponse
	if err := json.Unmarshal(raw, &fsResp); err != nil {
		return nil, fmt.Errorf("parse flaresolverr response: %w", err)
	}

	if fsResp.Status != "ok" {
		return nil, fmt.Errorf("flaresolverr error: %s", fsResp.Message)
	}

	if fsResp.Solution.Status != http.StatusOK {
		return nil, fmt.Errorf("flaresolverr upstream status: %d", fsResp.Solution.Status)
	}

	// Sync cookies returned by FlareSolverr back into the client
	if len(fsResp.Solution.Cookies) > 0 {
		c.mu.Lock()
		if c.cookies == nil {
			c.cookies = make(map[string]string)
		}
		for _, cookie := range fsResp.Solution.Cookies {
			if cookie.Name != "" && cookie.Value != "" {
				c.cookies[cookie.Name] = cookie.Value
			}
		}
		snapshot := normalizeCookies(c.cookies)
		cookiesPath := c.cookiesPath
		c.mu.Unlock()

		if cookiesPath == "" {
			cookiesPath = resolveCookiesFilePath("")
		}
		if err := persistCookiesToFile(cookiesPath, snapshot); err != nil {
			return nil, fmt.Errorf("persist cookies after flaresolverr: %w", err)
		}
	}

	return []byte(fsResp.Solution.Response), nil
}
