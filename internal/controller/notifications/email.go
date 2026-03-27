// Package notifications provides notification providers for VirtueStack Controller.
package notifications

import (
	"bytes"
	"context"
	"crypto/tls"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/shared/util"
)

//go:embed templates/*.html
var templateFS embed.FS

// templateNames is the list of email content templates to load.
var templateNames = []string{
	"vm-created",
	"vm-deleted",
	"vm-suspended",
	"backup-failed",
	"node-offline",
	"bandwidth-exceeded",
	"password-reset",
	"default",
}

// EmailConfig holds configuration for the email provider.
type EmailConfig struct {
	Enabled    bool
	Host       string
	Port       int
	Username   string
	Password   string
	From       string
	UseTLS     bool
	FromName   string
	RequireTLS bool // When true, enforce STARTTLS for non-465 ports (QG-02)
}

// EmailPayload contains data for an email notification.
type EmailPayload struct {
	To           string         `json:"to"`
	Subject      string         `json:"subject"`
	Template     string         `json:"template"`
	CustomerName string         `json:"customer_name,omitempty"`
	Data         map[string]any `json:"data,omitempty"`
}

// EmailProvider sends email notifications via SMTP.
type EmailProvider struct {
	config     EmailConfig
	logger     *slog.Logger
	templates  *template.Template
	templateMu sync.RWMutex
}

// emailTemplateData holds data passed to email templates.
type emailTemplateData struct {
	Subject      string
	CustomerName string
	Data         map[string]any
	Year         int
}

// NewEmailProvider creates a new EmailProvider with the given configuration.
func NewEmailProvider(config EmailConfig, logger *slog.Logger) (*EmailProvider, error) {
	if !config.Enabled {
		logger.Info("email provider disabled")
		return &EmailProvider{
			config: config,
			logger: logger.With("component", "email-provider"),
		}, nil
	}

	// Validate required fields
	if config.Host == "" {
		return nil, fmt.Errorf("SMTP host is required")
	}
	if config.From == "" {
		return nil, fmt.Errorf("SMTP from address is required")
	}

	// Set defaults
	if config.Port == 0 {
		config.Port = 587
	}
	if config.FromName == "" {
		config.FromName = "VirtueStack"
	}
	// Enable TLS by default for port 587
	if config.Port == 587 {
		config.UseTLS = true
	}

	// Warn operators who have credentials configured but haven't opted into RequireTLS
	// on any non-implicit-TLS port (i.e. not 465), where PlainAuth could send
	// credentials without encryption — including port 587 without STARTTLS.
	if config.Username != "" && !config.RequireTLS && config.Port != 465 {
		logger.Warn("SMTP credentials configured without RequireTLS; credentials may be sent in plaintext — set SMTP_REQUIRE_TLS=true to enforce STARTTLS",
			"port", config.Port)
	}

	provider := &EmailProvider{
		config: config,
		logger: logger.With("component", "email-provider"),
	}

	// Load templates
	if err := provider.loadTemplates(); err != nil {
		logger.Warn("failed to load email templates, using defaults",
			"error", err)
	}

	logger.Info("email provider initialized",
		"host", config.Host,
		"port", config.Port,
		"from", config.From)

	return provider, nil
}

// loadTemplates loads email templates from embedded files.
func (p *EmailProvider) loadTemplates() error {
	p.templateMu.Lock()
	defer p.templateMu.Unlock()

	baseContent, err := templateFS.ReadFile("templates/base.html")
	if err != nil {
		return fmt.Errorf("reading base template: %w", err)
	}

	tmpl, err := template.New("base").Parse(string(baseContent))
	if err != nil {
		return fmt.Errorf("parsing base template: %w", err)
	}

	for _, name := range templateNames {
		content, err := templateFS.ReadFile("templates/" + name + ".html")
		if err != nil {
			p.logger.Warn("failed to read template file", "template", name, "error", err)
			continue
		}
		if _, err := tmpl.New(name).Parse(string(content)); err != nil {
			p.logger.Warn("failed to parse template", "template", name, "error", err)
		}
	}

	p.templates = tmpl
	return nil
}

// smtpDialTimeout is the timeout applied when establishing an SMTP/TLS connection.
// A nonresponsive server would otherwise block the caller indefinitely.
const smtpDialTimeout = 10 * time.Second

// Send sends an email notification.
// The context controls cancellation; if it is already done when Send is called,
// the function returns immediately. The context deadline (if any) is also
// propagated to the underlying network connection so hung SMTP servers cannot
// block indefinitely.
func (p *EmailProvider) Send(ctx context.Context, payload *EmailPayload) error {
	if !p.config.Enabled {
		p.logger.Debug("email provider disabled, skipping send")
		return nil
	}

	// Check context before performing any I/O.
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled before sending email: %w", err)
	}

	if payload.To == "" {
		return fmt.Errorf("recipient email address is required")
	}

	// Validate recipient address to prevent header injection and malformed addresses.
	if _, err := mail.ParseAddress(payload.To); err != nil {
		return fmt.Errorf("invalid recipient address %q: %w", payload.To, err)
	}

	p.logger.Info("sending email",
		"to", util.MaskEmail(payload.To),
		"subject", payload.Subject,
		"template", payload.Template)

	// Render email body
	body, err := p.renderTemplate(payload)
	if err != nil {
		return fmt.Errorf("rendering email template: %w", err)
	}

	// Build email message
	from := p.config.From
	if p.config.FromName != "" {
		from = fmt.Sprintf("%s <%s>", p.config.FromName, p.config.From)
	}

	msg := p.buildMessage(from, payload.To, payload.Subject, body)

	// Send email, threading the context so the dial honours cancellation.
	if err := p.sendEmail(ctx, payload.To, msg); err != nil {
		return fmt.Errorf("sending email: %w", err)
	}

	p.logger.Info("email sent successfully",
		"to", util.MaskEmail(payload.To),
		"subject", payload.Subject)

	return nil
}

// renderTemplate renders the email template with the given data.
func (p *EmailProvider) renderTemplate(payload *EmailPayload) (string, error) {
	p.templateMu.RLock()
	defer p.templateMu.RUnlock()

	if p.templates == nil {
		// Return a simple default if templates not loaded
		return fmt.Sprintf("<p>Hello %s,</p><p>%s</p>", payload.CustomerName, payload.Subject), nil
	}

	data := emailTemplateData{
		Subject:      payload.Subject,
		CustomerName: payload.CustomerName,
		Data:         payload.Data,
		Year:         time.Now().Year(),
	}

	// If customer name is empty, use a generic greeting
	if data.CustomerName == "" {
		data.CustomerName = "Customer"
	}

	var buf bytes.Buffer
	templateName := payload.Template
	if templateName == "" {
		templateName = "default"
	}

	// Try to execute the specific template, fall back to default
	tmpl := p.templates.Lookup(templateName)
	if tmpl == nil {
		tmpl = p.templates.Lookup("default")
		if tmpl == nil {
			return "", fmt.Errorf("no template found for %s", templateName)
		}
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

// sanitizeHeader removes CR and LF characters from a header value to prevent
// SMTP header injection attacks. An attacker-controlled subject containing
// "\r\n" could otherwise inject arbitrary headers into the outgoing message.
func sanitizeHeader(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}

// buildMessage builds the raw email message.
func (p *EmailProvider) buildMessage(from, to, subject, body string) string {
	msg := fmt.Sprintf("From: %s\r\n", sanitizeHeader(from))
	msg += fmt.Sprintf("To: %s\r\n", sanitizeHeader(to))
	msg += fmt.Sprintf("Subject: %s\r\n", sanitizeHeader(subject))
	msg += "MIME-version: 1.0;\r\n"
	msg += "Content-Type: text/html; charset=\"UTF-8\";\r\n"
	msg += "\r\n"
	msg += body

	return msg
}

// sendEmail sends the email via SMTP.
// ctx is threaded into the underlying dial so that cancellation and deadlines
// are honoured; a nonresponsive server will not block past the context deadline
// or smtpDialTimeout, whichever is shorter.
func (p *EmailProvider) sendEmail(ctx context.Context, to, msg string) error {
	addr := fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)

	var auth smtp.Auth
	if p.config.Username != "" && p.config.Password != "" {
		auth = smtp.PlainAuth("", p.config.Username, p.config.Password, p.config.Host)
	}

	// When RequireTLS is set, force STARTTLS for any non-implicit-TLS port (not 465).
	// sendWithSTARTTLS returns an error if the server does not advertise STARTTLS,
	// which prevents PlainAuth credentials from being sent in cleartext.
	if p.config.RequireTLS && p.config.Port != 465 {
		return p.sendWithSTARTTLS(ctx, addr, auth, to, msg)
	}

	// For TLS connections (port 587), we need to handle STARTTLS
	if p.config.UseTLS && p.config.Port == 587 {
		return p.sendWithSTARTTLS(ctx, addr, auth, to, msg)
	}

	// For SSL connections (port 465) or non-TLS
	if p.config.Port == 465 {
		return p.sendWithTLS(ctx, addr, auth, to, msg)
	}

	// Standard SMTP (port 25): dial with timeout so a nonresponsive server does
	// not block the caller indefinitely.
	dialCtx, cancel := context.WithTimeout(ctx, smtpDialTimeout)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("connecting to SMTP server: %w", err)
	}

	client, err := smtp.NewClient(conn, p.config.Host)
	if err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			p.logger.Debug("failed to close connection after SMTP client error", "error", closeErr)
		}
		return fmt.Errorf("creating SMTP client: %w", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			p.logger.Debug("failed to close SMTP client", "error", err)
		}
	}()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authenticating: %w", err)
		}
	}
	if err := client.Mail(p.config.From); err != nil {
		return fmt.Errorf("setting sender: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("setting recipient: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("preparing data: %w", err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			p.logger.Debug("failed to close SMTP data writer", "error", err)
		}
	}()
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("writing message: %w", err)
	}
	return client.Quit()
}

// sendWithSTARTTLS sends email using STARTTLS.
// ctx is used to apply a dial timeout so that a nonresponsive server does not
// block the caller indefinitely.
func (p *EmailProvider) sendWithSTARTTLS(ctx context.Context, addr string, auth smtp.Auth, to, msg string) error {
	// Dial with a bounded timeout; smtp.Dial has no timeout of its own and would
	// block forever if the server is unresponsive.
	dialCtx, cancel := context.WithTimeout(ctx, smtpDialTimeout)
	defer cancel()

	netConn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("connecting to SMTP server: %w", err)
	}

	conn, err := smtp.NewClient(netConn, p.config.Host)
	if err != nil {
		if closeErr := netConn.Close(); closeErr != nil {
			p.logger.Debug("failed to close connection after SMTP client error", "error", closeErr)
		}
		return fmt.Errorf("creating SMTP client: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			p.logger.Debug("failed to close SMTP connection", "error", err)
		}
	}()

	// Send EHLO
	if err := conn.Hello("virtuestack.local"); err != nil {
		return fmt.Errorf("sending EHLO: %w", err)
	}

	// Start TLS
	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         p.config.Host,
	}
	if err := conn.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("starting TLS: %w", err)
	}

	// Authenticate
	if auth != nil {
		if err := conn.Auth(auth); err != nil {
			return fmt.Errorf("authenticating: %w", err)
		}
	}

	// Send email
	if err := conn.Mail(p.config.From); err != nil {
		return fmt.Errorf("setting sender: %w", err)
	}

	if err := conn.Rcpt(to); err != nil {
		return fmt.Errorf("setting recipient: %w", err)
	}

	// Send data
	w, err := conn.Data()
	if err != nil {
		return fmt.Errorf("preparing data: %w", err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			p.logger.Debug("failed to close SMTP data writer", "error", err)
		}
	}()

	_, err = w.Write([]byte(msg))
	if err != nil {
		return fmt.Errorf("writing message: %w", err)
	}

	return conn.Quit()
}

// sendWithTLS sends email using implicit TLS (port 465).
// ctx is used to apply a dial timeout so that a nonresponsive server does not
// block the caller indefinitely.
func (p *EmailProvider) sendWithTLS(ctx context.Context, addr string, auth smtp.Auth, to, msg string) error {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         p.config.Host,
	}

	// Dial with a bounded timeout; tls.Dial has no timeout of its own and would
	// block forever if the server is unresponsive.
	dialCtx, cancel := context.WithTimeout(ctx, smtpDialTimeout)
	defer cancel()

	netConn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("connecting to SMTP server with TLS: %w", err)
	}

	conn := tls.Client(netConn, tlsConfig)
	if err := conn.HandshakeContext(dialCtx); err != nil {
		if closeErr := netConn.Close(); closeErr != nil {
			p.logger.Debug("failed to close connection after TLS handshake error", "error", closeErr)
		}
		return fmt.Errorf("TLS handshake failed: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			p.logger.Debug("failed to close TLS connection", "error", err)
		}
	}()

	client, err := smtp.NewClient(conn, p.config.Host)
	if err != nil {
		return fmt.Errorf("creating SMTP client: %w", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			p.logger.Debug("failed to close SMTP client", "error", err)
		}
	}()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authenticating: %w", err)
		}
	}

	if err := client.Mail(p.config.From); err != nil {
		return fmt.Errorf("setting sender: %w", err)
	}

	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("setting recipient: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("preparing data: %w", err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			p.logger.Debug("failed to close SMTP data writer", "error", err)
		}
	}()

	_, err = w.Write([]byte(msg))
	if err != nil {
		return fmt.Errorf("writing message: %w", err)
	}

	return client.Quit()
}

// IsEnabled returns whether the email provider is enabled.
func (p *EmailProvider) IsEnabled() bool {
	return p.config.Enabled
}

// LoadEmailConfigFromEnv loads email configuration from environment variables.
func LoadEmailConfigFromEnv() EmailConfig {
	smtpRequireTLS := os.Getenv("SMTP_REQUIRE_TLS")
	return EmailConfig{
		Enabled:    os.Getenv("NOTIFICATION_EMAIL_ENABLED") == "true",
		Host:       os.Getenv("NOTIFICATION_EMAIL_SMTP_HOST"),
		Port:       parsePort(os.Getenv("NOTIFICATION_EMAIL_SMTP_PORT")),
		Username:   os.Getenv("NOTIFICATION_EMAIL_USERNAME"),
		Password:   os.Getenv("NOTIFICATION_EMAIL_PASSWORD"),
		From:       os.Getenv("NOTIFICATION_EMAIL_FROM"),
		UseTLS:     true,
		RequireTLS: smtpRequireTLS == "true" || smtpRequireTLS == "1",
	}
}

func parsePort(s string) int {
	if s == "" {
		return 587
	}
	// strconv.Atoi is preferred over fmt.Sscanf because it returns an error when
	// the entire string cannot be parsed as an integer, whereas Sscanf silently
	// ignores trailing non-numeric characters and does not report partial matches.
	port, err := strconv.Atoi(s)
	if err != nil || port <= 0 || port > 65535 {
		return 587
	}
	return port
}
