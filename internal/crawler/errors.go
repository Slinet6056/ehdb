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
	if match := temporaryBanPattern.FindString(trimmed); match != "" {
		return match, true
	}

	abnormalMarkers := []string{
		"your ip address has been temporarily banned",
		"ban expires in",
		"attention required",
		"just a moment",
		"checking your browser before accessing",
		"captcha",
		"cloudflare",
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
