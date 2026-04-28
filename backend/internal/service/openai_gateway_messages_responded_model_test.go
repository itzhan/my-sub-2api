//go:build unit

package service

import "testing"

func TestChooseRespondedAnthropicModel(t *testing.T) {
	cases := []struct {
		name         string
		original     string
		normalized   string
		billing      string
		want         string
	}{
		// Cross-family mapping: client sent claude, group maps to gpt → return gpt.
		{"claude_to_gpt", "claude-sonnet-4", "claude-sonnet-4", "gpt-5.4", "gpt-5.4"},
		{"claude_haiku_to_gpt5_mini", "claude-haiku-4", "claude-haiku-4", "gpt-5.4-mini", "gpt-5.4-mini"},

		// Suffix-only normalization: gpt-5.4-xhigh → gpt-5.4 → keep original (xhigh suffix).
		{"gpt5_xhigh_preserved", "gpt-5.4-xhigh", "gpt-5.4", "gpt-5.4", "gpt-5.4-xhigh"},
		{"gpt5_high_preserved", "gpt-5.5-high", "gpt-5.5", "gpt-5.5", "gpt-5.5-high"},

		// No mapping at all: keep original.
		{"gpt_passthrough", "gpt-5.4", "gpt-5.4", "gpt-5.4", "gpt-5.4"},
		{"claude_passthrough", "claude-sonnet-4", "claude-sonnet-4", "claude-sonnet-4", "claude-sonnet-4"},

		// Edge: empty billing → fall back to original.
		{"empty_billing", "claude-sonnet-4", "claude-sonnet-4", "", "claude-sonnet-4"},

		// Edge: empty normalized but original != billing → treat as mapping.
		{"empty_normalized_with_mapping", "claude-sonnet-4", "", "gpt-5.4", "gpt-5.4"},

		// Edge: whitespace-only → trim then evaluate.
		{"whitespace_normalized", "claude-sonnet-4", "  ", "gpt-5.4", "gpt-5.4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := chooseRespondedAnthropicModel(tc.original, tc.normalized, tc.billing)
			if got != tc.want {
				t.Errorf("chooseRespondedAnthropicModel(%q,%q,%q)=%q, want %q",
					tc.original, tc.normalized, tc.billing, got, tc.want)
			}
		})
	}
}
