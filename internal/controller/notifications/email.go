// Package notifications provides notification providers for VirtueStack Controller.
package notifications

import (
	"bytes"
	"context"
	"crypto/tls"
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
	// on port 25, where PlainAuth would send credentials without encryption.
	if config.Username != "" && !config.RequireTLS && config.Port == 25 {
		logger.Warn("SMTP credentials configured without RequireTLS; credentials may be sent in plaintext on port 25 — set SMTP_REQUIRE_TLS=true to enforce STARTTLS")
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

// loadTemplates loads email templates from the templates directory.
func (p *EmailProvider) loadTemplates() error {
	p.templateMu.Lock()
	defer p.templateMu.Unlock()

	// Define base template with common layout
	baseTemplate := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Subject}}</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f5; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f5; padding: 40px 0;">
        <tr>
            <td align="center">
                <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="background-color: #ffffff; border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.1);">
                    <!-- Header -->
                    <tr>
                        <td style="padding: 32px 40px; background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%); border-radius: 8px 8px 0 0;">
                            <h1 style="margin: 0; color: #ffffff; font-size: 24px; font-weight: 600;">VirtueStack</h1>
                        </td>
                    </tr>
                    <!-- Content -->
                    <tr>
                        <td style="padding: 40px;">
                            {{template "content" .}}
                        </td>
                    </tr>
                    <!-- Footer -->
                    <tr>
                        <td style="padding: 24px 40px; background-color: #f9fafb; border-top: 1px solid #e5e7eb; border-radius: 0 0 8px 8px;">
                            <p style="margin: 0; color: #6b7280; font-size: 14px;">
                                © {{.Year}} VirtueStack. All rights reserved.
                            </p>
                        </td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>`

	// Define template content for each event type
	templateContents := map[string]string{
		"vm-created": `
{{define "content"}}
<h2 style="margin: 0 0 16px 0; color: #111827; font-size: 20px;">🎉 Your VM is Ready!</h2>
<p style="margin: 0 0 16px 0; color: #374151; font-size: 16px;">Hello {{.CustomerName}},</p>
<p style="margin: 0 0 24px 0; color: #374151; font-size: 16px;">Your virtual machine has been successfully created and is now ready for use.</p>
<table role="presentation" cellpadding="0" cellspacing="0" style="width: 100%; margin-bottom: 24px;">
    <tr>
        <td style="padding: 12px 16px; background-color: #f3f4f6; border-radius: 4px;">
            <p style="margin: 0; color: #6b7280; font-size: 14px;">VM Hostname</p>
            <p style="margin: 4px 0 0 0; color: #111827; font-size: 16px; font-weight: 600;">{{.Data.hostname}}</p>
        </td>
    </tr>
</table>
<p style="margin: 0; color: #374151; font-size: 16px;">You can access your VM through the <a href="#" style="color: #4f46e5;">customer portal</a>.</p>
{{end}}`,

		"vm-deleted": `
{{define "content"}}
<h2 style="margin: 0 0 16px 0; color: #111827; font-size: 20px;">🗑️ VM Deleted</h2>
<p style="margin: 0 0 16px 0; color: #374151; font-size: 16px;">Hello {{.CustomerName}},</p>
<p style="margin: 0 0 24px 0; color: #374151; font-size: 16px;">Your virtual machine has been successfully deleted as requested.</p>
<table role="presentation" cellpadding="0" cellspacing="0" style="width: 100%; margin-bottom: 24px;">
    <tr>
        <td style="padding: 12px 16px; background-color: #fef2f2; border-radius: 4px; border-left: 4px solid #ef4444;">
            <p style="margin: 0; color: #6b7280; font-size: 14px;">Deleted VM</p>
            <p style="margin: 4px 0 0 0; color: #111827; font-size: 16px; font-weight: 600;">{{.Data.hostname}}</p>
        </td>
    </tr>
</table>
<p style="margin: 0; color: #374151; font-size: 16px;">All associated data has been permanently removed. If this was unexpected, please contact support immediately.</p>
{{end}}`,

		"vm-suspended": `
{{define "content"}}
<h2 style="margin: 0 0 16px 0; color: #111827; font-size: 20px;">⏸️ VM Suspended</h2>
<p style="margin: 0 0 16px 0; color: #374151; font-size: 16px;">Hello {{.CustomerName}},</p>
<p style="margin: 0 0 24px 0; color: #374151; font-size: 16px;">Your virtual machine has been suspended.</p>
<table role="presentation" cellpadding="0" cellspacing="0" style="width: 100%; margin-bottom: 24px;">
    <tr>
        <td style="padding: 12px 16px; background-color: #fffbeb; border-radius: 4px; border-left: 4px solid #f59e0b;">
            <p style="margin: 0; color: #6b7280; font-size: 14px;">Suspended VM</p>
            <p style="margin: 4px 0 0 0; color: #111827; font-size: 16px; font-weight: 600;">{{.Data.hostname}}</p>
            {{if .Data.reason}}<p style="margin: 8px 0 0 0; color: #92400e; font-size: 14px;"><strong>Reason:</strong> {{.Data.reason}}</p>{{end}}
        </td>
    </tr>
</table>
<p style="margin: 0; color: #374151; font-size: 16px;">Please contact support to resolve this issue.</p>
{{end}}`,

		"backup-failed": `
{{define "content"}}
<h2 style="margin: 0 0 16px 0; color: #111827; font-size: 20px;">⚠️ Backup Failed</h2>
<p style="margin: 0 0 16px 0; color: #374151; font-size: 16px;">Hello {{.CustomerName}},</p>
<p style="margin: 0 0 24px 0; color: #374151; font-size: 16px;">A backup operation for your virtual machine has failed.</p>
<table role="presentation" cellpadding="0" cellspacing="0" style="width: 100%; margin-bottom: 24px;">
    <tr>
        <td style="padding: 12px 16px; background-color: #fef2f2; border-radius: 4px; border-left: 4px solid #ef4444;">
            <p style="margin: 0; color: #6b7280; font-size: 14px;">VM</p>
            <p style="margin: 4px 0 0 0; color: #111827; font-size: 16px; font-weight: 600;">{{.Data.hostname}}</p>
            {{if .Data.error}}<p style="margin: 8px 0 0 0; color: #dc2626; font-size: 14px;"><strong>Error:</strong> {{.Data.error}}</p>{{end}}
        </td>
    </tr>
</table>
<p style="margin: 0; color: #374151; font-size: 16px;">Our team has been notified and will investigate. You may retry the backup from the control panel.</p>
{{end}}`,

		"node-offline": `
{{define "content"}}
<h2 style="margin: 0 0 16px 0; color: #111827; font-size: 20px;">🔴 Node Offline Alert</h2>
<p style="margin: 0 0 16px 0; color: #374151; font-size: 16px;">Hello {{.CustomerName}},</p>
<p style="margin: 0 0 24px 0; color: #374151; font-size: 16px;">A hypervisor node has gone offline. This may affect VM availability.</p>
<table role="presentation" cellpadding="0" cellspacing="0" style="width: 100%; margin-bottom: 24px;">
    <tr>
        <td style="padding: 12px 16px; background-color: #fef2f2; border-radius: 4px; border-left: 4px solid #ef4444;">
            <p style="margin: 0; color: #6b7280; font-size: 14px;">Node</p>
            <p style="margin: 4px 0 0 0; color: #111827; font-size: 16px; font-weight: 600;">{{.Data.node_name}}</p>
        </td>
    </tr>
</table>
<p style="margin: 0; color: #374151; font-size: 16px;">Our infrastructure team has been notified and is working to restore service.</p>
{{end}}`,

		"bandwidth-exceeded": `
{{define "content"}}
<h2 style="margin: 0 0 16px 0; color: #111827; font-size: 20px;">📊 Bandwidth Limit Exceeded</h2>
<p style="margin: 0 0 16px 0; color: #374151; font-size: 16px;">Hello {{.CustomerName}},</p>
<p style="margin: 0 0 24px 0; color: #374151; font-size: 16px;">Your VM has exceeded its monthly bandwidth allocation.</p>
<table role="presentation" cellpadding="0" cellspacing="0" style="width: 100%; margin-bottom: 24px;">
    <tr>
        <td style="padding: 12px 16px; background-color: #fffbeb; border-radius: 4px; border-left: 4px solid #f59e0b;">
            <p style="margin: 0; color: #6b7280; font-size: 14px;">VM</p>
            <p style="margin: 4px 0 0 0; color: #111827; font-size: 16px; font-weight: 600;">{{.Data.hostname}}</p>
            <p style="margin: 8px 0 0 0; color: #374151; font-size: 14px;"><strong>Used:</strong> {{.Data.used_gb}} GB / <strong>Limit:</strong> {{.Data.limit_gb}} GB</p>
        </td>
    </tr>
</table>
<p style="margin: 0; color: #374151; font-size: 16px;">Your network speed may be throttled until the next billing cycle. Consider upgrading your plan for more bandwidth.</p>
{{end}}`,

		"default": `
{{define "content"}}
<h2 style="margin: 0 0 16px 0; color: #111827; font-size: 20px;">Notification</h2>
<p style="margin: 0 0 16px 0; color: #374151; font-size: 16px;">Hello {{.CustomerName}},</p>
<p style="margin: 0 0 24px 0; color: #374151; font-size: 16px;">You have a new notification from VirtueStack.</p>
{{end}}`,
	}

	// Parse base template
	tmpl, err := template.New("base").Parse(baseTemplate)
	if err != nil {
		return fmt.Errorf("parsing base template: %w", err)
	}

	// Add content templates
	for name, content := range templateContents {
		_, err := tmpl.New(name).Parse(content)
		if err != nil {
			p.logger.Warn("failed to parse template",
				"template", name,
				"error", err)
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
		conn.Close()
		return fmt.Errorf("creating SMTP client: %w", err)
	}
	defer client.Close()

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
	defer w.Close()
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
		netConn.Close()
		return fmt.Errorf("creating SMTP client: %w", err)
	}
	defer conn.Close()

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
	defer w.Close()

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
		netConn.Close()
		return fmt.Errorf("TLS handshake failed: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, p.config.Host)
	if err != nil {
		return fmt.Errorf("creating SMTP client: %w", err)
	}
	defer client.Close()

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
	defer w.Close()

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
	return EmailConfig{
		Enabled:  os.Getenv("NOTIFICATION_EMAIL_ENABLED") == "true",
		Host:     os.Getenv("NOTIFICATION_EMAIL_SMTP_HOST"),
		Port:     parsePort(os.Getenv("NOTIFICATION_EMAIL_SMTP_PORT")),
		Username: os.Getenv("NOTIFICATION_EMAIL_USERNAME"),
		Password: os.Getenv("NOTIFICATION_EMAIL_PASSWORD"),
		From:     os.Getenv("NOTIFICATION_EMAIL_FROM"),
		UseTLS:   true,
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
