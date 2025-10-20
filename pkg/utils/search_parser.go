package utils

import (
	"regexp"
	"strings"
)

// SearchTerm represents a parsed search term
type SearchTerm struct {
	Type     string   // "phrase", "tag", "tag_prefix", "wildcard", "exclude", "or", "keyword"
	Value    string   // The actual search value
	Values   []string // For OR terms
	IsExact  bool     // For tags: whether it's an exact match ($)
	Original string   // Original input
}

// SearchQuery represents the parsed search query
type SearchQuery struct {
	Phrases     []string   // Exact phrases in quotes
	Tags        []string   // Exact tag matches (with $)
	TagPrefixes []string   // Tag prefix searches (without $)
	Wildcards   []string   // Wildcard terms
	Excludes    []string   // Excluded terms
	OrGroups    [][]string // OR groups (each group is list of alternatives)
	Keywords    []string   // Regular keywords for title search
}

// ParseSearchKeyword parses the search keyword string into structured query
func ParseSearchKeyword(keyword string) *SearchQuery {
	query := &SearchQuery{
		Phrases:     []string{},
		Tags:        []string{},
		TagPrefixes: []string{},
		Wildcards:   []string{},
		Excludes:    []string{},
		OrGroups:    [][]string{},
		Keywords:    []string{},
	}

	if keyword == "" {
		return query
	}

	remaining := keyword

	// Collect all OR terms into a single OR group
	var allOrTerms []string

	// Step 1: Extract OR tags with quoted values first (e.g., ~namespace:"value")
	// Must process these BEFORE regular tags to avoid conflicts
	orTagWithQuotesRegex := regexp.MustCompile(`~(\w+):"([^"]+)"`)
	orTagMatches := orTagWithQuotesRegex.FindAllStringSubmatch(remaining, -1)
	for _, match := range orTagMatches {
		if len(match) > 2 {
			namespace := strings.TrimSpace(match[1])
			value := strings.TrimSpace(match[2])

			// Check if value ends with $ (exact match indicator)
			isExact := strings.HasSuffix(value, "$")
			if isExact {
				value = strings.TrimSuffix(value, "$")
			}

			// Normalize the tag
			tagToken := namespace + ":" + value
			normalizedTag := NormalizeTag(tagToken)

			// Validate tag format (must be namespace:value)
			parts := strings.SplitN(normalizedTag, ":", 2)
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				// Collect to allOrTerms instead of adding as separate OR group
				if isExact {
					allOrTerms = append(allOrTerms, "TAG_EXACT:"+normalizedTag)
				} else {
					allOrTerms = append(allOrTerms, "TAG_PREFIX:"+normalizedTag)
				}
			}
		}
	}
	// Remove processed OR tags from remaining (use regex to replace all matches)
	remaining = orTagWithQuotesRegex.ReplaceAllString(remaining, " ")

	// Step 2: Extract excluded tags with quoted values (e.g., -namespace:"value")
	excludeTagWithQuotesRegex := regexp.MustCompile(`-(\w+):"([^"]+)"`)
	excludeTagMatches := excludeTagWithQuotesRegex.FindAllStringSubmatch(remaining, -1)
	for _, match := range excludeTagMatches {
		if len(match) > 2 {
			namespace := strings.TrimSpace(match[1])
			value := strings.TrimSpace(match[2])

			// Check if value ends with $ (exact match indicator)
			isExact := strings.HasSuffix(value, "$")
			if isExact {
				value = strings.TrimSuffix(value, "$")
			}

			// Normalize the tag
			tagToken := namespace + ":" + value
			normalizedTag := NormalizeTag(tagToken)

			// Validate tag format (must be namespace:value)
			parts := strings.SplitN(normalizedTag, ":", 2)
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				// For excluded tags, add with special prefix
				if isExact {
					query.Excludes = append(query.Excludes, "TAG_EXACT:"+normalizedTag)
				} else {
					query.Excludes = append(query.Excludes, "TAG_PREFIX:"+normalizedTag)
				}
			}
		}
	}
	// Remove processed exclude tags from remaining (use regex to replace all matches)
	remaining = excludeTagWithQuotesRegex.ReplaceAllString(remaining, " ")

	// Step 2: Extract excluded quoted phrases first (e.g., -"phrase")
	excludeQuotedRegex := regexp.MustCompile(`-"([^"]+)"`)
	excludeQuotedMatches := excludeQuotedRegex.FindAllStringSubmatch(remaining, -1)
	for _, match := range excludeQuotedMatches {
		if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
			query.Excludes = append(query.Excludes, strings.TrimSpace(match[1]))
		}
	}
	// Remove excluded quoted phrases from remaining
	remaining = excludeQuotedRegex.ReplaceAllString(remaining, " ")

	// Step 2.5: Extract OR quoted phrases (e.g., ~"phrase")
	orQuotedRegex := regexp.MustCompile(`~"([^"]+)"`)
	orQuotedMatches := orQuotedRegex.FindAllStringSubmatch(remaining, -1)
	for _, match := range orQuotedMatches {
		if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
			// Collect to allOrTerms instead of adding as separate OR group
			allOrTerms = append(allOrTerms, strings.TrimSpace(match[1]))
		}
	}
	// Remove OR quoted phrases from remaining
	remaining = orQuotedRegex.ReplaceAllString(remaining, " ")

	// Step 3: Extract regular tags with quoted values (e.g., namespace:"value" or namespace:"value$")
	// Process these AFTER OR/exclude tags to avoid conflicts
	tagWithQuotesRegex := regexp.MustCompile(`(\w+):"([^"]+)"`)
	tagMatches := tagWithQuotesRegex.FindAllStringSubmatch(remaining, -1)
	for _, match := range tagMatches {
		if len(match) > 2 {
			namespace := strings.TrimSpace(match[1])
			value := strings.TrimSpace(match[2])

			// Check if value ends with $ (exact match indicator)
			isExact := strings.HasSuffix(value, "$")
			if isExact {
				value = strings.TrimSuffix(value, "$")
			}

			// Normalize the tag
			tagToken := namespace + ":" + value
			normalizedTag := NormalizeTag(tagToken)

			// Validate tag format (must be namespace:value)
			parts := strings.SplitN(normalizedTag, ":", 2)
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				if isExact {
					query.Tags = append(query.Tags, normalizedTag)
				} else {
					query.TagPrefixes = append(query.TagPrefixes, normalizedTag)
				}
			}
		}
	}
	// Remove processed tags from remaining
	remaining = tagWithQuotesRegex.ReplaceAllString(remaining, " ")

	// Step 5: Extract regular quoted phrases (not tags, not excluded, not OR)
	phraseRegex := regexp.MustCompile(`"([^"]+)"`)
	phrases := phraseRegex.FindAllStringSubmatch(remaining, -1)
	for _, match := range phrases {
		if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
			query.Phrases = append(query.Phrases, strings.TrimSpace(match[1]))
		}
	}
	// Remove quoted phrases from remaining
	remaining = phraseRegex.ReplaceAllString(remaining, " ")

	// Step 6: Split remaining by spaces
	tokens := strings.Fields(remaining)

	// Step 7: Parse each token
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		// Check for exclude (-)
		if strings.HasPrefix(token, "-") && len(token) > 1 {
			excludeTerm := strings.TrimPrefix(token, "-")

			// Check if it's an excluded tag (contains :)
			if strings.Contains(excludeTerm, ":") {
				// Check if it's an exact tag match (ends with $)
				isExact := strings.HasSuffix(excludeTerm, "$")
				tagToken := excludeTerm
				if isExact {
					tagToken = strings.TrimSuffix(excludeTerm, "$")
				}

				// Normalize the tag (expand shortcuts)
				normalizedTag := NormalizeTag(tagToken)

				// Validate tag format (must be namespace:value)
				parts := strings.SplitN(normalizedTag, ":", 2)
				if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
					if isExact {
						query.Excludes = append(query.Excludes, "TAG_EXACT:"+normalizedTag)
					} else {
						query.Excludes = append(query.Excludes, "TAG_PREFIX:"+normalizedTag)
					}
				}
			} else {
				// Regular exclude term (for title)
				query.Excludes = append(query.Excludes, excludeTerm)
			}
			continue
		}

		// Check for OR (~)
		if strings.HasPrefix(token, "~") && len(token) > 1 {
			orTerm := strings.TrimPrefix(token, "~")
			// Split by comma or just use single term
			orTerms := strings.Split(orTerm, ",")
			for _, t := range orTerms {
				t = strings.TrimSpace(t)
				if t != "" {
					// Check if it's a tag (contains :)
					if strings.Contains(t, ":") {
						// Check if it's an exact tag match (ends with $)
						isExact := strings.HasSuffix(t, "$")
						tagToken := t
						if isExact {
							tagToken = strings.TrimSuffix(t, "$")
						}

						// Normalize the tag (expand shortcuts)
						normalizedTag := NormalizeTag(tagToken)

						// Validate tag format (must be namespace:value)
						parts := strings.SplitN(normalizedTag, ":", 2)
						if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
							if isExact {
								allOrTerms = append(allOrTerms, "TAG_EXACT:"+normalizedTag)
							} else {
								allOrTerms = append(allOrTerms, "TAG_PREFIX:"+normalizedTag)
							}
						}
					} else {
						// Add regular term to allOrTerms
						allOrTerms = append(allOrTerms, t)
					}
				}
			}
			continue
		}

		// Check for tag with colon (:)
		if strings.Contains(token, ":") {
			// Check if it's an exact tag match (ends with $)
			isExact := strings.HasSuffix(token, "$")
			tagToken := token
			if isExact {
				tagToken = strings.TrimSuffix(token, "$")
			}

			// Normalize the tag (expand shortcuts)
			normalizedTag := NormalizeTag(tagToken)

			// Validate tag format (must be namespace:value)
			parts := strings.SplitN(normalizedTag, ":", 2)
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				if isExact {
					query.Tags = append(query.Tags, normalizedTag)
				} else {
					query.TagPrefixes = append(query.TagPrefixes, normalizedTag)
				}
			}
			continue
		}

		// Check for wildcard (* or %)
		if strings.Contains(token, "*") || strings.Contains(token, "%") {
			// Convert * to %
			wildcardTerm := strings.ReplaceAll(token, "*", "%")
			query.Wildcards = append(query.Wildcards, wildcardTerm)
			continue
		}

		// Regular keyword (for title search)
		query.Keywords = append(query.Keywords, token)
	}

	// Add all collected OR terms as a single OR group
	if len(allOrTerms) > 0 {
		query.OrGroups = append(query.OrGroups, allOrTerms)
	}

	return query
}

// ExpandTagPrefixes queries the database to find matching tags for prefix searches
// This should be called with a database connection
func ExpandTagPrefixes(tagPrefixes []string, tagFetcher func(string) []string) []string {
	var expandedTags []string
	for _, prefix := range tagPrefixes {
		matches := tagFetcher(prefix)
		expandedTags = append(expandedTags, matches...)
	}
	return expandedTags
}
