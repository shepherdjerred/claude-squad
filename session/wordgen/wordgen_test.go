package wordgen

import (
	"regexp"
	"testing"
)

func TestGenerate(t *testing.T) {
	// Test that Generate returns a non-empty string
	result := Generate()
	if result == "" {
		t.Fatal("Generate() returned empty string")
	}

	// Test format: should match "word_word" pattern
	pattern := regexp.MustCompile(`^[a-z]+_[a-z]+$`)
	if !pattern.MatchString(result) {
		t.Errorf("Generate() = %q, expected format 'word_word'", result)
	}
}

func TestGenerateFormat(t *testing.T) {
	pattern := regexp.MustCompile(`^[a-z]+_[a-z]+$`)

	for i := 0; i < 10; i++ {
		result := Generate()
		if !pattern.MatchString(result) {
			t.Errorf("Generate() iteration %d = %q, does not match pattern", i, result)
		}
	}
}

func TestGenerateUniqueness(t *testing.T) {
	// Generate multiple suffixes and check for some variety
	// With 50 adjectives and 50 nouns, we should see different results
	results := make(map[string]bool)
	iterations := 100

	for i := 0; i < iterations; i++ {
		result := Generate()
		results[result] = true
	}

	// We should have at least some unique values
	// (Not all will be unique due to randomness, but should have > 50%)
	uniqueCount := len(results)
	if uniqueCount < iterations/2 {
		t.Errorf("Generate() produced %d unique values out of %d iterations, expected more variety", uniqueCount, iterations)
	}
}

func TestGenerateComponents(t *testing.T) {
	result := Generate()
	if result == "" {
		t.Fatal("Generate() returned empty string")
	}

	// Split and verify both parts exist
	var found bool

	// Check if adjective is from the list
	for _, adj := range adjectives {
		if len(result) > len(adj) && result[:len(adj)] == adj {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Generate() = %q, adjective not found in adjectives list", result)
	}

	// Check if noun is from the list
	found = false
	for _, noun := range nouns {
		if len(result) > len(noun) && result[len(result)-len(noun):] == noun {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Generate() = %q, noun not found in nouns list", result)
	}
}

func TestWordLists(t *testing.T) {
	// Verify adjectives list
	if len(adjectives) == 0 {
		t.Error("adjectives list is empty")
	}

	// All adjectives should be lowercase
	for _, adj := range adjectives {
		if adj != adj {
			t.Errorf("adjective %q is not lowercase", adj)
		}
		if len(adj) < 3 {
			t.Errorf("adjective %q is too short (< 3 chars)", adj)
		}
	}

	// Verify nouns list
	if len(nouns) == 0 {
		t.Error("nouns list is empty")
	}

	// All nouns should be lowercase
	for _, noun := range nouns {
		if noun != noun {
			t.Errorf("noun %q is not lowercase", noun)
		}
		if len(noun) < 3 {
			t.Errorf("noun %q is too short (< 3 chars)", noun)
		}
	}
}

func TestSelectRandom(t *testing.T) {
	testWords := []string{"alpha", "beta", "gamma"}

	// Test successful selection
	result, err := selectRandom(testWords)
	if err != nil {
		t.Fatalf("selectRandom() error = %v", err)
	}

	// Verify result is from the list
	found := false
	for _, word := range testWords {
		if result == word {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("selectRandom() = %q, not in input list", result)
	}

	// Test empty list
	_, err = selectRandom([]string{})
	if err == nil {
		t.Error("selectRandom(empty list) expected error, got nil")
	}
}
