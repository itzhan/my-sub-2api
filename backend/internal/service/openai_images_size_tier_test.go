//go:build unit

package service

import "testing"

func TestNormalizeOpenAIImageSizeTier(t *testing.T) {
	cases := []struct {
		size string
		want string
	}{
		// 1K 档（≤1024 且 ≤1.1MP）
		{"1024x1024", "1K"},
		{"1024x768", "1K"},
		{"768x1024", "1K"},
		{"1024X1024", "1K"}, // 大小写不敏感
		{" 1024x1024 ", "1K"},

		// 2K 档（1.1MP < pixels < 2.5MP 且长边 < 2048）
		{"1536x1024", "2K"},
		{"1024x1536", "2K"},
		{"1792x1024", "2K"},
		{"1024x1792", "2K"},
		{"1536x1536", "2K"}, // 2.36MP, 长边 1536 → 2K
		{"", "2K"},          // 空 → auto
		{"auto", "2K"},
		{"AUTO", "2K"},
		{"unknown_format", "2K"}, // 解析失败 → 2K

		// 4K 档（≥2.5MP 或 长边 ≥2048）
		{"2048x2048", "4K"}, // 4.2MP
		{"2048x1024", "4K"}, // 长边=2048
		{"1024x2048", "4K"}, // 长边=2048
		{"3840x2160", "4K"}, // 真 4K, 8.3MP
		{"2160x3840", "4K"}, // 4K 竖版
		{"1792x1792", "4K"}, // 3.2MP → 4K
		{"3840x3840", "4K"},
		{"4096x4096", "4K"},
	}
	for _, tc := range cases {
		got := normalizeOpenAIImageSizeTier(tc.size)
		if got != tc.want {
			t.Errorf("normalizeOpenAIImageSizeTier(%q) = %q, want %q", tc.size, got, tc.want)
		}
	}
}

func TestParseOpenAIImageSizeDimensions(t *testing.T) {
	cases := []struct {
		s         string
		w, h      int
		wantOK    bool
	}{
		{"1024x1024", 1024, 1024, true},
		{"3840x2160", 3840, 2160, true},
		{"1024X1024", 0, 0, false}, // parser is case-sensitive; normalize lower-cases first
		{"  768x1024  ", 768, 1024, true},
		{"1024 x 1024", 1024, 1024, true},
		{"", 0, 0, false},
		{"auto", 0, 0, false},
		{"1024", 0, 0, false},
		{"1024x", 0, 0, false},
		{"x1024", 0, 0, false},
		{"-1x1024", 0, 0, false},
		{"abcxdef", 0, 0, false},
	}
	for _, tc := range cases {
		w, h, ok := parseOpenAIImageSizeDimensions(tc.s)
		if ok != tc.wantOK || (ok && (w != tc.w || h != tc.h)) {
			t.Errorf("parseOpenAIImageSizeDimensions(%q) = (%d, %d, %v), want (%d, %d, %v)", tc.s, w, h, ok, tc.w, tc.h, tc.wantOK)
		}
	}
}
