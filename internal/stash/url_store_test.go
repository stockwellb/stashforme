package stash

import "testing"

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "adds https scheme",
			input:    "example.com",
			expected: "https://example.com",
		},
		{
			name:     "keeps existing https",
			input:    "https://example.com",
			expected: "https://example.com",
		},
		{
			name:     "keeps existing http",
			input:    "http://example.com",
			expected: "http://example.com",
		},
		{
			name:     "lowercases host",
			input:    "https://EXAMPLE.COM",
			expected: "https://example.com",
		},
		{
			name:     "removes trailing slash",
			input:    "https://example.com/path/",
			expected: "https://example.com/path",
		},
		{
			name:     "keeps root path",
			input:    "https://example.com/",
			expected: "https://example.com/",
		},
		{
			name:     "removes fragment",
			input:    "https://example.com/page#section",
			expected: "https://example.com/page",
		},
		{
			name:     "removes default https port",
			input:    "https://example.com:443/path",
			expected: "https://example.com/path",
		},
		{
			name:     "removes default http port",
			input:    "http://example.com:80/path",
			expected: "http://example.com/path",
		},
		{
			name:     "keeps non-default port",
			input:    "https://example.com:8080/path",
			expected: "https://example.com:8080/path",
		},
		{
			name:     "preserves query params",
			input:    "https://example.com/search?q=test&page=1",
			expected: "https://example.com/search?q=test&page=1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeURL(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeURL(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestHashURL(t *testing.T) {
	// Same URL should produce same hash
	url1 := "https://example.com/page"
	url2 := "https://example.com/page"

	hash1 := HashURL(url1)
	hash2 := HashURL(url2)

	if hash1 != hash2 {
		t.Errorf("Same URL produced different hashes: %q vs %q", hash1, hash2)
	}

	// Different URLs should produce different hashes
	url3 := "https://example.com/other"
	hash3 := HashURL(url3)

	if hash1 == hash3 {
		t.Errorf("Different URLs produced same hash: %q", hash1)
	}

	// Hash should be 64 characters (SHA256 hex)
	if len(hash1) != 64 {
		t.Errorf("Hash length = %d, want 64", len(hash1))
	}
}
