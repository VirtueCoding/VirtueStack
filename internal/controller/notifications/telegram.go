package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// TelegramConfig holds configuration for the Telegram provider.
type TelegramConfig struct {
	Enabled     bool
	BotToken    string
	AdminChatIDs []string
	Timeout     time.Duration
}

// TelegramPayload contains data for a Telegram notification.
type TelegramPayload struct {
	Message   string `json:"message"`
	ParseMode string `json:"parse_mode,omitempty"`
	ChatID    string `json:"chat_id,omitempty"`
}

// TelegramProvider sends notifications via Telegram Bot API.
type TelegramProvider struct {
	config TelegramConfig
	client *http.Client
	logger *slog.Logger
}

// telegramMessage represents a Telegram message to send.
type telegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

// telegramResponse represents a response from Telegram API.
type telegramResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
	ErrorCode   int    `json:"error_code,omitempty"`
}

// NewTelegramProvider creates a new TelegramProvider with the given configuration.
func NewTelegramProvider(config TelegramConfig, logger *slog.Logger) (*TelegramProvider, error) {
	if !config.Enabled {
		logger.Info("telegram provider disabled")
		return &TelegramProvider{
			config: config,
			logger: logger.With("component", "telegram-provider"),
		}, nil
	}

	// Validate required fields
	if config.BotToken == "" {
		return nil, fmt.Errorf("Telegram bot token is required")
	}
	if len(config.AdminChatIDs) == 0 {
		return nil, fmt.Errorf("at least one admin chat ID is required")
	}

	// Set defaults
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	provider := &TelegramProvider{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		logger: logger.With("component", "telegram-provider"),
	}

	logger.Info("telegram provider initialized",
		"admin_chats", len(config.AdminChatIDs))

	return provider, nil
}

// Send sends a Telegram notification to all configured admin chat IDs.
func (p *TelegramProvider) Send(ctx context.Context, payload *TelegramPayload) error {
	if !p.config.Enabled {
		p.logger.Debug("telegram provider disabled, skipping send")
		return nil
	}

	if payload.Message == "" {
		return fmt.Errorf("message is required")
	}

	// Determine target chat IDs
	chatIDs := p.config.AdminChatIDs
	if payload.ChatID != "" {
		chatIDs = []string{payload.ChatID}
	}

	parseMode := payload.ParseMode
	if parseMode == "" {
		parseMode = "Markdown"
	}

	p.logger.Info("sending telegram notification",
		"chat_count", len(chatIDs))

	var errors []error
	for _, chatID := range chatIDs {
		if err := p.sendToChat(ctx, chatID, payload.Message, parseMode); err != nil {
			p.logger.Error("failed to send telegram message",
				"chat_id", chatID,
				"error", err)
			errors = append(errors, fmt.Errorf("chat %s: %w", chatID, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to send to %d chats: %v", len(errors), errors[0])
	}

	return nil
}

// sendToChat sends a message to a specific chat.
func (p *TelegramProvider) sendToChat(ctx context.Context, chatID, message, parseMode string) error {
	msg := telegramMessage{
		ChatID:    chatID,
		Text:      message,
		ParseMode: parseMode,
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling message: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", p.config.BotToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	var result telegramResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	if !result.OK {
		return fmt.Errorf("telegram API error (code %d): %s", result.ErrorCode, result.Description)
	}

	p.logger.Debug("telegram message sent",
		"chat_id", chatID)

	return nil
}

// SendAlert sends an alert notification to all admin chats.
func (p *TelegramProvider) SendAlert(ctx context.Context, title, message string) error {
	fullMessage := fmt.Sprintf("🚨 *%s*\n\n%s\n\n_Time: %s_",
		EscapeMarkdown(title),
		EscapeMarkdown(message),
		time.Now().Format(time.RFC3339))

	return p.Send(ctx, &TelegramPayload{
		Message: fullMessage,
	})
}

// SendInfo sends an informational notification.
func (p *TelegramProvider) SendInfo(ctx context.Context, title, message string) error {
	fullMessage := fmt.Sprintf("ℹ️ *%s*\n\n%s\n\n_Time: %s_",
		EscapeMarkdown(title),
		EscapeMarkdown(message),
		time.Now().Format(time.RFC3339))

	return p.Send(ctx, &TelegramPayload{
		Message: fullMessage,
	})
}

// IsEnabled returns whether the Telegram provider is enabled.
func (p *TelegramProvider) IsEnabled() bool {
	return p.config.Enabled
}

// EscapeMarkdown escapes special characters for Telegram Markdown formatting.
func EscapeMarkdown(text string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(text)
}

// EscapeMarkdownV2 escapes special characters for Telegram MarkdownV2 formatting.
func EscapeMarkdownV2(text string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(text)
}

// FormatBold returns text formatted as bold in Telegram Markdown.
func FormatBold(text string) string {
	return fmt.Sprintf("*%s*", EscapeMarkdown(text))
}

// FormatItalic returns text formatted as italic in Telegram Markdown.
func FormatItalic(text string) string {
	return fmt.Sprintf("_%s_", EscapeMarkdown(text))
}

// FormatCode returns text formatted as inline code in Telegram Markdown.
func FormatCode(text string) string {
	return fmt.Sprintf("`%s`", text)
}

// FormatCodeBlock returns text formatted as a code block in Telegram Markdown.
func FormatCodeBlock(text string) string {
	return fmt.Sprintf("```\n%s\n```", text)
}

// FormatLink returns a hyperlink formatted for Telegram Markdown.
func FormatLink(text, url string) string {
	return fmt.Sprintf("[%s](%s)", text, url)
}

// LoadTelegramConfigFromEnv loads Telegram configuration from environment variables.
func LoadTelegramConfigFromEnv() TelegramConfig {
	enabled := os.Getenv("NOTIFICATION_TELEGRAM_ENABLED") == "true"
	botToken := os.Getenv("NOTIFICATION_TELEGRAM_BOT_TOKEN")
	chatIDsStr := os.Getenv("NOTIFICATION_TELEGRAM_ADMIN_CHAT_IDS")

	var chatIDs []string
	if chatIDsStr != "" {
		// Split by comma
		for _, id := range strings.Split(chatIDsStr, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				chatIDs = append(chatIDs, id)
			}
		}
	}

	return TelegramConfig{
		Enabled:      enabled,
		BotToken:     botToken,
		AdminChatIDs: chatIDs,
		Timeout:      10 * time.Second,
	}
}