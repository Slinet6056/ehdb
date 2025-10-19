package utils

import (
	"fmt"
	"regexp"
	"strings"
)

// FormatSQL formats a SQL query with placeholders replaced by actual values for debugging
// This function converts PostgreSQL placeholders ($1, $2, etc.) into their actual values
// making the query directly executable for debugging purposes.
// It also cleans up the SQL by removing newlines, tabs, and extra spaces.
//
// Example:
//
//	query := "SELECT * FROM users WHERE id = $1 AND name = $2"
//	formatted := FormatSQL(query, 123, "John")
//	Returns: "SELECT * FROM users WHERE id = 123 AND name = 'John'"
func FormatSQL(query string, args ...interface{}) string {
	result := query
	for i, arg := range args {
		placeholder := fmt.Sprintf("$%d", i+1)
		var value string
		switch v := arg.(type) {
		case string:
			// Escape single quotes for SQL strings
			value = fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''"))
		case []string:
			// Convert Go string slice to PostgreSQL ARRAY syntax
			quoted := make([]string, len(v))
			for j, s := range v {
				quoted[j] = fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", "''"))
			}
			value = fmt.Sprintf("ARRAY[%s]", strings.Join(quoted, ", "))
		case []int:
			// Convert Go int slice to PostgreSQL ARRAY syntax
			strValues := make([]string, len(v))
			for j, n := range v {
				strValues[j] = fmt.Sprintf("%d", n)
			}
			value = fmt.Sprintf("ARRAY[%s]", strings.Join(strValues, ", "))
		case int, int64, int32, int16, int8:
			value = fmt.Sprintf("%v", v)
		case uint, uint64, uint32, uint16, uint8:
			value = fmt.Sprintf("%v", v)
		case float32, float64:
			value = fmt.Sprintf("%v", v)
		case bool:
			if v {
				value = "true"
			} else {
				value = "false"
			}
		case nil:
			value = "NULL"
		default:
			// Fallback for other types
			value = fmt.Sprintf("'%v'", v)
		}
		result = strings.Replace(result, placeholder, value, 1)
	}

	// Clean up whitespace: remove newlines, tabs, and compress multiple spaces
	result = strings.ReplaceAll(result, "\n", " ")
	result = strings.ReplaceAll(result, "\t", " ")
	result = strings.ReplaceAll(result, "\r", " ")

	// Replace multiple consecutive spaces with a single space
	spaceRegex := regexp.MustCompile(`\s+`)
	result = spaceRegex.ReplaceAllString(result, " ")

	// Trim leading and trailing spaces
	result = strings.TrimSpace(result)

	return result
}
