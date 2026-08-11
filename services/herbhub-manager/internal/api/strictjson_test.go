package api

import "testing"

func TestValidateNoDuplicateJSONKeysRejectsRecursiveDuplicates(t *testing.T) {
	cases := []string{
		`{"config":{"enabled":true,"enabled":false},"confirm_enable":true}`,
		`{"config":{"plants":{"basil":{"threshold_percent":30,"threshold_percent":31}}},"confirm_enable":false}`,
		`{"config":{"plants":{"basil":{},"basil":{}}},"confirm_enable":false}`,
	}
	for _, c := range cases {
		if err := validateNoDuplicateJSONKeys([]byte(c)); err == nil {
			t.Fatalf("expected duplicate key error for %s", c)
		}
	}
}

func TestValidateNoDuplicateJSONKeysAllowsDistinct(t *testing.T) {
	if err := validateNoDuplicateJSONKeys([]byte(`{"config":{"enabled":false,"plants":{"basil":{"enabled":true}}},"confirm_enable":false}`)); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
