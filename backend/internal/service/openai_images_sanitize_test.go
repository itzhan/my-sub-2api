//go:build unit

package service

import "testing"

func TestSanitizeOpenAIImagesRequest_Style(t *testing.T) {
	for _, in := range []string{"vivid", "natural", "anything"} {
		req := &OpenAIImagesRequest{Style: in}
		sanitizeOpenAIImagesRequest(req)
		if req.Style != "" {
			t.Errorf("Style %q should be cleared, got %q", in, req.Style)
		}
	}
}

func TestSanitizeOpenAIImagesRequest_OutputFormat(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"png", "png"},
		{"PNG", "png"},
		{"jpeg", "jpeg"},
		{"JPEG", "jpeg"},
		{"jpg", "jpeg"}, // jpg → jpeg
		{"webp", ""},    // dropped (silently downgraded to default png)
		{"WEBP", ""},
		{"bmp", ""},     // unsupported
		{"", ""},        // empty stays empty
		{"  ", ""},      // whitespace stays empty
	}
	for _, tc := range cases {
		req := &OpenAIImagesRequest{OutputFormat: tc.in}
		sanitizeOpenAIImagesRequest(req)
		if req.OutputFormat != tc.want {
			t.Errorf("OutputFormat %q → %q, want %q", tc.in, req.OutputFormat, tc.want)
		}
	}
}

func TestSanitizeOpenAIImagesRequest_Background(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"opaque", "opaque"},
		{"OPAQUE", "opaque"},
		{"auto", "auto"},
		{"transparent", "auto"}, // rewritten
		{"TRANSPARENT", "auto"},
		{"unknown", ""},
		{"", ""},
	}
	for _, tc := range cases {
		req := &OpenAIImagesRequest{Background: tc.in}
		sanitizeOpenAIImagesRequest(req)
		if req.Background != tc.want {
			t.Errorf("Background %q → %q, want %q", tc.in, req.Background, tc.want)
		}
	}
}

func TestSanitizeOpenAIImagesRequest_Quality(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"low", "low"},
		{"medium", "medium"},
		{"high", "high"},
		{"auto", "auto"},
		{"HIGH", "high"},
		{"hd", ""},   // unsupported
		{"max", ""},  // unsupported
		{"", ""},
	}
	for _, tc := range cases {
		req := &OpenAIImagesRequest{Quality: tc.in}
		sanitizeOpenAIImagesRequest(req)
		if req.Quality != tc.want {
			t.Errorf("Quality %q → %q, want %q", tc.in, req.Quality, tc.want)
		}
	}
}

func TestSanitizeOpenAIImagesRequest_Moderation(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"auto", "auto"},
		{"low", "low"},
		{"high", ""},  // unsupported
		{"strict", ""},
		{"", ""},
	}
	for _, tc := range cases {
		req := &OpenAIImagesRequest{Moderation: tc.in}
		sanitizeOpenAIImagesRequest(req)
		if req.Moderation != tc.want {
			t.Errorf("Moderation %q → %q, want %q", tc.in, req.Moderation, tc.want)
		}
	}
}

func TestSanitizeOpenAIImagesRequest_OutputCompression(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{50, 50},
		{0, 0},
		{100, 100},
		{-10, 0},
		{200, 100},
	}
	for _, tc := range cases {
		v := tc.in
		req := &OpenAIImagesRequest{OutputCompression: &v}
		sanitizeOpenAIImagesRequest(req)
		if req.OutputCompression == nil {
			t.Errorf("OutputCompression %d → nil, want %d", tc.in, tc.want)
			continue
		}
		if *req.OutputCompression != tc.want {
			t.Errorf("OutputCompression %d → %d, want %d", tc.in, *req.OutputCompression, tc.want)
		}
	}
}

func TestSanitizeOpenAIImagesRequest_NilSafe(t *testing.T) {
	sanitizeOpenAIImagesRequest(nil) // should not panic
}
