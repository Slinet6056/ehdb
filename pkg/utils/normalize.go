package utils

import (
	"strings"
)

// shortMap maps tag namespace shortcuts to full names (matching E-Hentai convention)
var shortMap = map[string]string{
	"a":      "artist",
	"c":      "character",
	"char":   "character",
	"cos":    "cosplayer",
	"f":      "female",
	"g":      "group",
	"circle": "group",
	"l":      "language",
	"lang":   "language",
	"loc":    "location",
	"m":      "male",
	"x":      "mixed",
	"o":      "other",
	"p":      "parody",
	"series": "parody",
	"r":      "reclass",
}

// NormalizeTag normalizes a tag by expanding shortcuts and converting to lowercase
func NormalizeTag(tag string) string {
	// Trim whitespace
	tag = strings.TrimSpace(tag)

	// Convert to lowercase
	tag = strings.ToLower(tag)

	// Replace multiple spaces with single space
	tag = strings.Join(strings.Fields(tag), " ")

	// Expand namespace shortcuts (e.g., "f:rape" -> "female:rape")
	if strings.Contains(tag, ":") {
		parts := strings.SplitN(tag, ":", 2)
		if len(parts) == 2 {
			namespace := parts[0]
			pattern := parts[1]
			if fullNamespace, ok := shortMap[namespace]; ok {
				return fullNamespace + ":" + pattern
			}
		}
	}

	return tag
}

// CategoryMap maps category bit flags to category names
var CategoryMap = map[int]string{
	1:    "Misc",
	2:    "Doujinshi",
	4:    "Manga",
	8:    "Artist CG",
	16:   "Game CG",
	32:   "Image Set",
	64:   "Cosplay",
	128:  "Asian Porn",
	256:  "Non-H",
	512:  "Western",
	1024: "Private",
}

// GetCategoriesFromBits converts bit flags to category names
func GetCategoriesFromBits(bits int) []string {
	var categories []string
	for bit, name := range CategoryMap {
		if bits&bit != 0 {
			categories = append(categories, name)
		}
	}
	return categories
}
