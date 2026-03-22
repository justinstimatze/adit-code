package diff

import (
	"testing"
)

func TestValidateRef(t *testing.T) {
	tests := []struct {
		ref   string
		valid bool
	}{
		{"HEAD~1", true},
		{"main", true},
		{"abc123", true},
		{"v1.0.0", true},
		{"-flag", false},
		{"--exec", false},
	}
	for _, tt := range tests {
		err := ValidateRef(tt.ref)
		if tt.valid && err != nil {
			t.Errorf("ValidateRef(%q) returned error: %v", tt.ref, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("ValidateRef(%q) should have returned error", tt.ref)
		}
	}
}
