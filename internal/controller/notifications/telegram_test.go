package notifications

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestEscapeMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no special chars",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "underscore",
			input: "hello_world",
			want:  "hello\\_world",
		},
		{
			name:  "asterisk",
			input: "hello*world",
			want:  "hello\\*world",
		},
		{
			name:  "bracket",
			input: "hello[world",
			want:  "hello\\[world",
		},
		{
			name:  "backtick",
			input: "hello`world",
			want:  "hello\\`world",
		},
		{
			name:  "multiple special chars",
			input: "_bold_ and *italic* and [link]",
			want:  "\\_bold\\_ and \\*italic\\* and \\[link]",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapeMarkdown(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEscapeMarkdownV2(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no special chars",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "underscore",
			input: "hello_world",
			want:  "hello\\_world",
		},
		{
			name:  "parentheses",
			input: "hello(world)",
			want:  "hello\\(world\\)",
		},
		{
			name:  "all special chars",
			input: "_*[]()~`>#+-=|{}.!",
			want:  "\\_\\*\\[\\]\\(\\)\\~\\`\\>\\#\\+\\-\\=\\|\\{\\}\\.\\!",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapeMarkdownV2(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatBold(t *testing.T) {
	got := FormatBold("test")
	assert.Equal(t, "*test*", got)
}

func TestFormatBold_WithSpecialChars(t *testing.T) {
	got := FormatBold("test_value")
	assert.Equal(t, "*test\\_value*", got)
}

func TestFormatItalic(t *testing.T) {
	got := FormatItalic("test")
	assert.Equal(t, "_test_", got)
}

func TestFormatItalic_WithSpecialChars(t *testing.T) {
	got := FormatItalic("test*value")
	assert.Equal(t, "_test\\*value_", got)
}

func TestFormatCode(t *testing.T) {
	got := FormatCode("test")
	assert.Equal(t, "`test`", got)
}

func TestFormatCode_StripsBackticks(t *testing.T) {
	got := FormatCode("te`st")
	assert.Equal(t, "`test`", got)
}

func TestFormatCodeBlock(t *testing.T) {
	got := FormatCodeBlock("line1\nline2")
	assert.Equal(t, "```\nline1\nline2\n```", got)
}

func TestFormatLink(t *testing.T) {
	got := FormatLink("Example", "https://example.com")
	assert.Equal(t, "[Example](https://example.com)", got)
}

func TestLoadTelegramConfigFromEnv_Disabled(t *testing.T) {
	// When env vars are not set, config should be disabled
	config := LoadTelegramConfigFromEnv()
	assert.False(t, config.Enabled)
	assert.Empty(t, config.BotToken)
}

func TestNewTelegramProvider_Disabled(t *testing.T) {
	config := TelegramConfig{
		Enabled: false,
	}

	provider, err := NewTelegramProvider(config, testLogger())
	assert.NoError(t, err)
	assert.NotNil(t, provider)
	assert.False(t, provider.IsEnabled())
}

func TestNewTelegramProvider_MissingBotToken(t *testing.T) {
	config := TelegramConfig{
		Enabled: true,
	}

	_, err := NewTelegramProvider(config, testLogger())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bot token is required")
}

func TestNewTelegramProvider_MissingChatIDs(t *testing.T) {
	config := TelegramConfig{
		Enabled:  true,
		BotToken: "test-bot-token",
	}

	_, err := NewTelegramProvider(config, testLogger())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one admin chat ID")
}

func TestNewTelegramProvider_Valid(t *testing.T) {
	config := TelegramConfig{
		Enabled:      true,
		BotToken:     "test-bot-token",
		AdminChatIDs: []string{"12345"},
	}

	provider, err := NewTelegramProvider(config, testLogger())
	assert.NoError(t, err)
	assert.NotNil(t, provider)
	assert.True(t, provider.IsEnabled())
}

func TestTelegramProvider_Send_Disabled(t *testing.T) {
	provider := &TelegramProvider{
		config: TelegramConfig{Enabled: false},
		logger: testLogger(),
	}

	err := provider.Send(nil, &TelegramPayload{Message: "test"})
	assert.NoError(t, err)
}

func TestTelegramProvider_Send_EmptyMessage(t *testing.T) {
	provider := &TelegramProvider{
		config: TelegramConfig{Enabled: true},
		logger: testLogger(),
	}

	err := provider.Send(nil, &TelegramPayload{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "message is required")
}
