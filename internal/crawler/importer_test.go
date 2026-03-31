package crawler

import (
	"reflect"
	"testing"
)

func TestPrepareTagsForUpsert(t *testing.T) {
	t.Run("deduplicates sorts and removes empty tags", func(t *testing.T) {
		input := []string{
			"artist:zeta",
			"",
			"character:alice",
			"artist:zeta",
			"group:circle",
			"character:alice",
		}

		got := prepareTagsForUpsert(input)
		want := []string{
			"artist:zeta",
			"character:alice",
			"group:circle",
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("expected %v, got %v", want, got)
		}
	})

	t.Run("returns nil for empty input", func(t *testing.T) {
		got := prepareTagsForUpsert(nil)
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})
}
