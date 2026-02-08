package crawler

import (
	"errors"
	"strings"
)

var (
	// ErrAuthRequired indicates crawler request cannot access target resource due to invalid cookie or insufficient account permissions.
	ErrAuthRequired = errors.New("authentication required or insufficient permissions")
)

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
