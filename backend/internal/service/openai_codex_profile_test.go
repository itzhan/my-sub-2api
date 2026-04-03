package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveOpenAICodexProfileByCandidates(t *testing.T) {
	tests := []struct {
		name           string
		candidates     []string
		wantOriginator string
		wantUserAgent  string
	}{
		{
			name:           "desktop alias maps to desktop profile",
			candidates:     []string{"Codex Desktop/1.2.3"},
			wantOriginator: "codex_chatgpt_desktop",
			wantUserAgent:  "codex_chatgpt_desktop/1.2.3",
		},
		{
			name:           "vscode originator maps to vscode profile",
			candidates:     []string{"codex_vscode"},
			wantOriginator: "codex_vscode",
			wantUserAgent:  "codex_vscode/" + codexCLIVersion,
		},
		{
			name:           "unknown custom ua falls back to cli_rs",
			candidates:     []string{"my-custom-agent/9.9"},
			wantOriginator: "codex_cli_rs",
			wantUserAgent:  "codex_cli_rs/" + codexCLIVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := resolveOpenAICodexProfileByCandidates(tt.candidates...)
			require.Equal(t, tt.wantOriginator, profile.Originator)
			require.Equal(t, tt.wantUserAgent, profile.UserAgent)
		})
	}
}

func TestOpenAIBuildUpstreamRequestAPIKeyAlwaysUsesCodexProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "Codex Desktop/1.2.3")

	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Security: config.SecurityConfig{
				URLAllowlist: config.URLAllowlistConfig{Enabled: false},
			},
		},
	}
	account := &Account{
		Type:        AccountTypeAPIKey,
		Platform:    PlatformOpenAI,
		Credentials: map[string]any{"base_url": "https://api.openai.com"},
	}

	req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte(`{"model":"gpt-5"}`), "token", false, "", false)
	require.NoError(t, err)
	require.Equal(t, "codex_chatgpt_desktop", req.Header.Get("originator"))
	require.Equal(t, "codex_chatgpt_desktop/1.2.3", req.Header.Get("User-Agent"))
}

func TestBuildOpenAIWSHeadersAPIKeyAlwaysUsesCodexProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "unit-test-agent/1.0")
	c.Request.Header.Set("originator", "codex_vscode")

	svc := &OpenAIGatewayService{}
	account := &Account{
		Type:     AccountTypeAPIKey,
		Platform: PlatformOpenAI,
	}

	headers, _ := svc.buildOpenAIWSHeaders(c, account, "sk-test", OpenAIWSProtocolDecision{}, false, "", "", "")
	require.Equal(t, "codex_vscode", headers.Get("originator"))
	require.Equal(t, "codex_vscode/"+codexCLIVersion, headers.Get("User-Agent"))
}
