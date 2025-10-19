package utils

import "github.com/slinet/ehdb/internal/database"

// GetResponse creates a standard API response
func GetResponse(data interface{}, code int, message string, total *int64) database.APIResponse {
	return database.APIResponse{
		Data:       data,
		Code:       code,
		Message:    message,
		Total:      total,
		NextCursor: nil,
	}
}

// GetResponseWithCursor creates a standard API response with cursor for pagination
func GetResponseWithCursor(data interface{}, code int, message string, total *int64, nextCursor *string) database.APIResponse {
	return database.APIResponse{
		Data:       data,
		Code:       code,
		Message:    message,
		Total:      total,
		NextCursor: nextCursor,
	}
}
