package crawler

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	// ErrAuthRequired indicates crawler request cannot access target resource due to invalid cookie or insufficient account permissions.
	ErrAuthRequired = errors.New("authentication required or insufficient permissions")
	ErrAbnormalPage = errors.New("abnormal page response")
)

// TransientError indicates a temporary upstream failure (e.g. HTTP 5xx or 429)
// that is likely to succeed on retry. It carries the originating status code and
// an optional server-suggested wait duration parsed from the Retry-After header.
type TransientError struct {
	StatusCode int
	RetryAfter time.Duration // zero when the server did not provide Retry-After
}

func (e *TransientError) Error() string {
	if e == nil {
		return "transient upstream error"
	}

	return fmt.Sprintf("unexpected status code: %d", e.StatusCode)
}

// asTransientError reports whether err (or anything it wraps) is a TransientError.
func asTransientError(err error) (*TransientError, bool) {
	if err == nil {
		return nil, false
	}

	var te *TransientError
	if errors.As(err, &te) {
		return te, true
	}

	return nil, false
}

// isTransientStatusCode reports whether an HTTP status code represents a
// temporary upstream condition worth retrying with backoff.
func isTransientStatusCode(status int) bool {
	switch status {
	case 429, // Too Many Requests
		500, // Internal Server Error
		502, // Bad Gateway
		503, // Service Unavailable
		504: // Gateway Timeout
		return true
	default:
		return false
	}
}

type PartialBackfillError struct {
	Cause           error
	ImportedCount   int
	DiscoveredCount int
	MissingCount    int
	ResumeStart     time.Time
	ResumeEnd       time.Time
}

func (e *PartialBackfillError) Error() string {
	if e == nil {
		return "partial backfill interrupted"
	}

	return fmt.Sprintf(
		"partial backfill interrupted after importing %d of %d missing galleries (%d discovered total); rerun overlapping window with -start %s -end %s: %v",
		e.ImportedCount,
		e.MissingCount,
		e.DiscoveredCount,
		e.ResumeStart.UTC().Format(time.RFC3339),
		e.ResumeEnd.UTC().Format(time.RFC3339),
		e.Cause,
	)
}

func (e *PartialBackfillError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Cause
}

var temporaryBanPattern = regexp.MustCompile(`(?i)your ip address has been temporarily banned.*?ban expires in [^<\n]+`)

func extractTemporaryBanMessage(content string) (string, bool) {
	if content == "" {
		return "", false
	}

	match := temporaryBanPattern.FindString(content)
	if match == "" {
		return "", false
	}

	return strings.TrimSpace(match), true
}

func isTemporaryBanError(err error) bool {
	if err == nil {
		return false
	}

	_, ok := extractTemporaryBanMessage(err.Error())
	return ok
}

func isAuthFailureBody(body []byte) (string, bool) {
	content := strings.ToLower(string(body))

	markers := []string{
		"please stand by while we redirect you",
		"if you are not redirected within a few seconds",
		"your browser does not support inline frames",
		"you do not have sufficient privileges to access this page",
		"this page requires you to log on",
		"sad panda",
		"sadpanda",
	}

	for _, marker := range markers {
		if strings.Contains(content, marker) {
			return marker, true
		}
	}

	return "", false
}

func abnormalGalleryListPageReason(body []byte) (string, bool) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "received blank gallery list page", true
	}

	lowerContent := strings.ToLower(trimmed)
	if match, ok := extractTemporaryBanMessage(trimmed); ok {
		return match, true
	}

	abnormalMarkers := []string{
		"your ip address has been temporarily banned",
		"ban expires in",
		"attention required",
		"just a moment",
		"checking your browser before accessing",
		"captcha",
		"cf-browser-verification",
		"cf_chl_opt",
		"ddos-guard",
		"access denied",
	}

	for _, marker := range abnormalMarkers {
		if strings.Contains(lowerContent, marker) {
			return marker, true
		}
	}

	hasGalleryListStructure := strings.Contains(lowerContent, "searchnav") ||
		strings.Contains(lowerContent, "nexturl=") ||
		strings.Contains(lowerContent, "class=\"itg") ||
		strings.Contains(lowerContent, "class=\"gl1t") ||
		strings.Contains(lowerContent, "class=\"gl3t") ||
		strings.Contains(lowerContent, "posted_")

	if !hasGalleryListStructure {
		return "missing expected gallery list structure", true
	}

	return "", false
}

func suspectedAbnormalWebPageReason(body []byte) (string, bool) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "received blank page", true
	}

	if match, ok := extractTemporaryBanMessage(trimmed); ok {
		return match, true
	}

	lowerContent := strings.ToLower(trimmed)
	abnormalMarkers := []string{
		"your ip address has been temporarily banned",
		"ban expires in",
		"attention required",
		"just a moment",
		"checking your browser before accessing",
		"captcha",
		"cf-browser-verification",
		"cf_chl_opt",
		"ddos-guard",
		"access denied",
	}

	for _, marker := range abnormalMarkers {
		if strings.Contains(lowerContent, marker) {
			return marker, true
		}
	}

	return "", false
}

func abnormalTorrentListPageReason(body []byte) (string, bool) {
	if reason, ok := suspectedAbnormalWebPageReason(body); ok {
		return reason, true
	}

	lowerContent := strings.ToLower(strings.TrimSpace(string(body)))
	hasTorrentListStructure := strings.Contains(lowerContent, "gallerytorrents.php?gid=") ||
		strings.Contains(lowerContent, "gtid=")

	if !hasTorrentListStructure {
		return "missing expected torrent list structure", true
	}

	return "", false
}
