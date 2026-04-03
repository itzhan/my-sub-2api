package service

import (
	"regexp"
	"strings"
)

type openAICodexProfile struct {
	Name       string
	Originator string
	UserAgent  string
}

var openAICodexProfileDefaults = map[string]openAICodexProfile{
	"codex_cli_rs": {
		Name:       "codex_cli_rs",
		Originator: "codex_cli_rs",
		UserAgent:  "codex_cli_rs/" + codexCLIVersion,
	},
	"codex_vscode": {
		Name:       "codex_vscode",
		Originator: "codex_vscode",
		UserAgent:  "codex_vscode/" + codexCLIVersion,
	},
	"codex_app": {
		Name:       "codex_app",
		Originator: "codex_app",
		UserAgent:  "codex_app/" + codexCLIVersion,
	},
	"codex_chatgpt_desktop": {
		Name:       "codex_chatgpt_desktop",
		Originator: "codex_chatgpt_desktop",
		UserAgent:  "codex_chatgpt_desktop/" + codexCLIVersion,
	},
	"codex_atlas": {
		Name:       "codex_atlas",
		Originator: "codex_atlas",
		UserAgent:  "codex_atlas/" + codexCLIVersion,
	},
	"codex_exec": {
		Name:       "codex_exec",
		Originator: "codex_exec",
		UserAgent:  "codex_exec/" + codexCLIVersion,
	},
	"codex_sdk_ts": {
		Name:       "codex_sdk_ts",
		Originator: "codex_sdk_ts",
		UserAgent:  "codex_sdk_ts/" + codexCLIVersion,
	},
}

var openAICodexProfileAliases = []struct {
	match string
	name  string
}{
	{match: "codex_chatgpt_desktop", name: "codex_chatgpt_desktop"},
	{match: "codex desktop", name: "codex_chatgpt_desktop"},
	{match: "codex_vscode", name: "codex_vscode"},
	{match: "codex_cli_rs", name: "codex_cli_rs"},
	{match: "codex_app", name: "codex_app"},
	{match: "codex_atlas", name: "codex_atlas"},
	{match: "codex_exec", name: "codex_exec"},
	{match: "codex_sdk_ts", name: "codex_sdk_ts"},
}

var openAICodexProfileVersionRE = regexp.MustCompile(`(?i)(codex(?:_[a-z0-9]+)*|codex desktop)/([a-z0-9][a-z0-9.\-]*)`)

func resolveOpenAICodexProfile(account *Account, userAgent, originator string) openAICodexProfile {
	candidates := []string{
		originator,
		userAgent,
	}
	if account != nil {
		candidates = append([]string{account.GetOpenAIUserAgent()}, candidates...)
	}
	return resolveOpenAICodexProfileByCandidates(candidates...)
}

func resolveOpenAICodexProfileByCandidates(candidates ...string) openAICodexProfile {
	for _, raw := range candidates {
		if profile, ok := resolveNamedOpenAICodexProfile(raw); ok {
			return profile
		}
	}
	return openAICodexProfileDefaults["codex_cli_rs"]
}

func resolveNamedOpenAICodexProfile(raw string) (openAICodexProfile, bool) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return openAICodexProfile{}, false
	}

	for _, alias := range openAICodexProfileAliases {
		if !strings.Contains(normalized, alias.match) {
			continue
		}
		base, ok := openAICodexProfileDefaults[alias.name]
		if !ok {
			return openAICodexProfile{}, false
		}
		if version := extractOpenAICodexProfileVersion(raw); version != "" {
			base.UserAgent = base.Name + "/" + version
		}
		return base, true
	}

	return openAICodexProfile{}, false
}

func extractOpenAICodexProfileVersion(raw string) string {
	match := openAICodexProfileVersionRE.FindStringSubmatch(strings.TrimSpace(raw))
	if len(match) < 3 {
		return ""
	}
	return strings.TrimSpace(match[2])
}
