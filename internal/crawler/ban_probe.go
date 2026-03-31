package crawler

import (
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

func probeAPITemporaryBan(client *Client, logger *zap.Logger) (string, bool) {
	if client == nil {
		return "", false
	}

	requestBody, err := json.Marshal(map[string]interface{}{
		"method":    "gdata",
		"gidlist":   [][2]interface{}{},
		"namespace": 1,
	})
	if err != nil {
		if logger != nil {
			logger.Debug("failed to marshal API ban probe request", zap.Error(err))
		}
		return "", false
	}

	body, err := client.Post(client.apiURL(), requestBody)
	if err != nil {
		if reason, ok := extractTemporaryBanMessage(err.Error()); ok {
			return reason, true
		}

		if logger != nil {
			logger.Debug("API ban probe request failed", zap.Error(err))
		}
		return "", false
	}

	if reason, ok := extractTemporaryBanMessage(string(body)); ok {
		return reason, true
	}

	return "", false
}

func enrichAbnormalReasonWithAPIProbe(reason string, client *Client, logger *zap.Logger) string {
	apiReason, ok := probeAPITemporaryBan(client, logger)
	if !ok {
		return reason
	}

	if apiReason == reason {
		return reason
	}

	return fmt.Sprintf("%s (api probe: %s)", reason, apiReason)
}
