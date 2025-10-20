package utils

import (
	"reflect"
	"testing"
)

func TestParseSearchKeyword(t *testing.T) {
	tests := []struct {
		name     string
		keyword  string
		expected *SearchQuery
	}{
		{
			name:    "empty keyword",
			keyword: "",
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{},
				Keywords:    []string{},
			},
		},
		{
			name:    "single keyword",
			keyword: "ai",
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{},
				Keywords:    []string{"ai"},
			},
		},
		{
			name:    "multiple keywords",
			keyword: "ai generated",
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{},
				Keywords:    []string{"ai", "generated"},
			},
		},
		{
			name:    "exact phrase",
			keyword: `"ai generated"`,
			expected: &SearchQuery{
				Phrases:     []string{"ai generated"},
				Tags:        []string{},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{},
				Keywords:    []string{},
			},
		},
		{
			name:    "multiple phrases",
			keyword: `"ai generated" "ia generated"`,
			expected: &SearchQuery{
				Phrases:     []string{"ai generated", "ia generated"},
				Tags:        []string{},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{},
				Keywords:    []string{},
			},
		},
		{
			name:    "exact tag",
			keyword: "language:chinese$",
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{"language:chinese"},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{},
				Keywords:    []string{},
			},
		},
		{
			name:    "prefix tag",
			keyword: "female:big",
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{"female:big"},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{},
				Keywords:    []string{},
			},
		},
		{
			name:    "tag with shorthand",
			keyword: "f:big c:miku",
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{"female:big", "character:miku"},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{},
				Keywords:    []string{},
			},
		},
		{
			name:    "wildcard",
			keyword: "*girl girl*",
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{},
				Wildcards:   []string{"%girl", "girl%"},
				Excludes:    []string{},
				OrGroups:    [][]string{},
				Keywords:    []string{},
			},
		},
		{
			name:    "exclude",
			keyword: "-ai -generated",
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{"ai", "generated"},
				OrGroups:    [][]string{},
				Keywords:    []string{},
			},
		},
		{
			name:    "exclude tag without quotes",
			keyword: "-male:small -female:dragon$",
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{"TAG_PREFIX:male:small", "TAG_EXACT:female:dragon"},
				OrGroups:    [][]string{},
				Keywords:    []string{},
			},
		},
		{
			name:    "or group",
			keyword: "~chinese,japanese,english",
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{{"chinese", "japanese", "english"}},
				Keywords:    []string{},
			},
		},
		{
			name:    "or tag without quotes",
			keyword: "~male:small ~female:dragon$",
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{{"TAG_PREFIX:male:small", "TAG_EXACT:female:dragon"}},
				Keywords:    []string{},
			},
		},
		{
			name:    "complex query",
			keyword: `"Paint Lab" f:big -male:drugs parody:original$ ~Nurse,Akuma *no* Chinese`,
			expected: &SearchQuery{
				Phrases:     []string{"Paint Lab"},
				Tags:        []string{"parody:original"},
				TagPrefixes: []string{"female:big"},
				Wildcards:   []string{"%no%"},
				Excludes:    []string{"TAG_PREFIX:male:drugs"},
				OrGroups:    [][]string{{"Nurse", "Akuma"}},
				Keywords:    []string{"Chinese"},
			},
		},
		{
			name:    "invalid tag format",
			keyword: "notag invalid:",
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{},
				Keywords:    []string{"notag"},
			},
		},
		{
			name:    "tag with quoted value",
			keyword: `character:"dark magician girl"`,
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{"character:dark magician girl"},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{},
				Keywords:    []string{},
			},
		},
		{
			name:    "exact tag with quoted value",
			keyword: `character:"dark magician girl$"`,
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{"character:dark magician girl"},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{},
				Keywords:    []string{},
			},
		},
		{
			name:    "tag with quoted value using shorthand",
			keyword: `c:"dark magician girl"`,
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{"character:dark magician girl"},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{},
				Keywords:    []string{},
			},
		},
		{
			name:    "multiple tags with quoted values",
			keyword: `c:"dark magician girl" p:"zenless zone zero$"`,
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{"parody:zenless zone zero"},
				TagPrefixes: []string{"character:dark magician girl"},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{},
				Keywords:    []string{},
			},
		},
		{
			name:    "mix of quoted tags and regular phrases",
			keyword: `"Pixiv | Twitter" c:"dark magician girl" other:"mosaic censorship"`,
			expected: &SearchQuery{
				Phrases:     []string{"Pixiv | Twitter"},
				Tags:        []string{},
				TagPrefixes: []string{"character:dark magician girl", "other:mosaic censorship"},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{},
				Keywords:    []string{},
			},
		},
		{
			name:    "exclude with quoted phrase",
			keyword: `-"AI Generated" -"IA Generated"`,
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{"AI Generated", "IA Generated"},
				OrGroups:    [][]string{},
				Keywords:    []string{},
			},
		},
		{
			name:    "exclude with quoted tag",
			keyword: `-f:"big" -m:"small"`,
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{"TAG_PREFIX:female:big", "TAG_PREFIX:male:small"},
				OrGroups:    [][]string{},
				Keywords:    []string{},
			},
		},
		{
			name:    "exclude with exact quoted tag",
			keyword: `-other:"mosaic censorship$"`,
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{"TAG_EXACT:other:mosaic censorship"},
				OrGroups:    [][]string{},
				Keywords:    []string{},
			},
		},
		{
			name:    "OR with quoted phrase",
			keyword: `~"touhou project" ~"東方"`,
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{{"touhou project", "東方"}},
				Keywords:    []string{},
			},
		},
		{
			name:    "OR with quoted tags",
			keyword: `~f:"big breast" ~f:"big ass"`,
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{{"TAG_PREFIX:female:big breast", "TAG_PREFIX:female:big ass"}},
				Keywords:    []string{},
			},
		},
		{
			name:    "OR with exact quoted tags",
			keyword: `~language:"chinese$" ~language:"japanese$"`,
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{{"TAG_EXACT:language:chinese", "TAG_EXACT:language:japanese"}},
				Keywords:    []string{},
			},
		},
		{
			name:    "mixed OR with tags and phrases",
			keyword: `~f:"big breast" ~"東方" ~chinese`,
			expected: &SearchQuery{
				Phrases:     []string{},
				Tags:        []string{},
				TagPrefixes: []string{},
				Wildcards:   []string{},
				Excludes:    []string{},
				OrGroups:    [][]string{{"TAG_PREFIX:female:big breast", "東方", "chinese"}},
				Keywords:    []string{},
			},
		},
		{
			name:    "complex query with all features",
			keyword: `"Paint Lab" f:big f:whip$ -male:drugs parody:original$ ~Nurse,Akuma ~parody:original$ ~o:story *no* Chinese -"AI Generated"`,
			expected: &SearchQuery{
				Phrases:     []string{"Paint Lab"},
				Tags:        []string{"female:whip", "parody:original"},
				TagPrefixes: []string{"female:big"},
				Wildcards:   []string{"%no%"},
				Excludes:    []string{"AI Generated", "TAG_PREFIX:male:drugs"},
				OrGroups:    [][]string{{"Nurse", "Akuma", "TAG_EXACT:parody:original", "TAG_PREFIX:other:story"}},
				Keywords:    []string{"Chinese"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseSearchKeyword(tt.keyword)

			if !reflect.DeepEqual(result.Phrases, tt.expected.Phrases) {
				t.Errorf("Phrases mismatch: got %v, want %v", result.Phrases, tt.expected.Phrases)
			}
			if !reflect.DeepEqual(result.Tags, tt.expected.Tags) {
				t.Errorf("Tags mismatch: got %v, want %v", result.Tags, tt.expected.Tags)
			}
			if !reflect.DeepEqual(result.TagPrefixes, tt.expected.TagPrefixes) {
				t.Errorf("TagPrefixes mismatch: got %v, want %v", result.TagPrefixes, tt.expected.TagPrefixes)
			}
			if !reflect.DeepEqual(result.Wildcards, tt.expected.Wildcards) {
				t.Errorf("Wildcards mismatch: got %v, want %v", result.Wildcards, tt.expected.Wildcards)
			}
			if !reflect.DeepEqual(result.Excludes, tt.expected.Excludes) {
				t.Errorf("Excludes mismatch: got %v, want %v", result.Excludes, tt.expected.Excludes)
			}
			if !reflect.DeepEqual(result.OrGroups, tt.expected.OrGroups) {
				t.Errorf("OrGroups mismatch: got %v, want %v", result.OrGroups, tt.expected.OrGroups)
			}
			if !reflect.DeepEqual(result.Keywords, tt.expected.Keywords) {
				t.Errorf("Keywords mismatch: got %v, want %v", result.Keywords, tt.expected.Keywords)
			}
		})
	}
}

func TestNormalizeTag(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "already normalized",
			input:    "language:chinese",
			expected: "language:chinese",
		},
		{
			name:     "shorthand l",
			input:    "l:chinese",
			expected: "language:chinese",
		},
		{
			name:     "shorthand lang",
			input:    "lang:chinese",
			expected: "language:chinese",
		},
		{
			name:     "uppercase",
			input:    "LANGUAGE:CHINESE",
			expected: "language:chinese",
		},
		{
			name:     "mixed case with spaces",
			input:    "  Language:Chinese  ",
			expected: "language:chinese",
		},
		{
			name:     "no namespace",
			input:    "just a tag",
			expected: "just a tag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeTag(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeTag(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
