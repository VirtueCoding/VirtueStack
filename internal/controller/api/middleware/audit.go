// Package middleware provides HTTP middleware for the VirtueStack Controller.
package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuditLogger is the function signature for persisting an AuditEntry.
// Implementations should write the entry to the audit_logs table (or equivalent).
type AuditLogger func(ctx context.Context, entry *AuditEntry) error

// AuditEntry represents a single audit log record.
// It captures who did what to which resource, and whether it succeeded.
type AuditEntry struct {
	// ActorID is the identifier of the entity that performed the action.
	ActorID string

	// ActorType classifies the actor: "admin", "customer", "provisioning", or "system".
	ActorType string

	// ActorIP is the source IP address of the request.
	ActorIP string

	// Action is a dot-separated action identifier, e.g. "vm.start", "node.create".
	Action string

	// ResourceType is the kind of resource affected, e.g. "vm", "node", "customer".
	ResourceType string

	// ResourceID is the unique identifier of the affected resource.
	ResourceID string

	// Changes holds before/after values for mutation operations.
	Changes map[string]any

	// CorrelationID links this audit entry to the originating request.
	CorrelationID string

	// Success indicates whether the handler returned a 2xx status.
	Success bool

	// ErrorMessage holds the error message when Success is false.
	ErrorMessage string
}

// Audit returns a Gin middleware that records mutating HTTP requests in the
// audit system. Only POST, PUT, PATCH, and DELETE requests are logged;
// read-only methods (GET, HEAD, OPTIONS) are passed through without logging.
func Audit(logger AuditLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if isReadOnlyMethod(method) {
			c.Next()
			return
		}

		// Process the request first so we can capture the response status.
		c.Next()

		status := c.Writer.Status()
		success := status >= http.StatusOK && status < http.StatusMultipleChoices

		resourceType, resourceID := ExtractResourceFromPath(c.Request.URL.Path)
		action := MapMethodToAction(method, c.Request.URL.Path)

		entry := &AuditEntry{
			ActorID:       resolveActorID(c),
			ActorType:     resolveActorType(c),
			ActorIP:       c.ClientIP(),
			Action:        action,
			ResourceType:  resourceType,
			ResourceID:    resourceID,
			CorrelationID: GetCorrelationID(c),
			Success:       success,
		}

		if !success {
			entry.ErrorMessage = c.Errors.Last().Error()
		}

		if err := logger(c.Request.Context(), entry); err != nil {
			slog.Error("failed to persist audit log entry",
				"error", err,
				"action", entry.Action,
				"correlation_id", entry.CorrelationID,
			)
		}
	}
}

// ExtractResourceFromPath derives the resource type and resource ID from a
// URL path following the VirtueStack path convention.
//
// Examples:
//
//	"/api/v1/admin/vms/uuid-here/start" → ("vm", "uuid-here")
//	"/api/v1/customers/uuid-here"       → ("customer", "uuid-here")
//	"/api/v1/nodes"                     → ("node", "")
func ExtractResourceFromPath(path string) (resourceType, resourceID string) {
	parts := splitPath(path)
	if len(parts) == 0 {
		return "", ""
	}

	// Walk the path segments to find a known resource noun.
	for i, segment := range parts {
		singular, ok := resourceNoun(segment)
		if !ok {
			continue
		}

		resourceType = singular

		// The segment immediately after the resource noun is the ID (if present
		// and looks like a UUID or opaque identifier, not a sub-action).
		if i+1 < len(parts) {
			candidate := parts[i+1]
			if isIdentifier(candidate) {
				resourceID = candidate
			}
		}

		return resourceType, resourceID
	}

	return "", ""
}

// MapMethodToAction produces an audit action string from an HTTP method and path.
// The action follows the "<resource>.<verb>" convention used by VirtueStack.
//
// Examples:
//
//	POST /vms         → "vm.create"
//	DELETE /vms/{id}  → "vm.delete"
//	POST /vms/{id}/start → "vm.start"
func MapMethodToAction(method, path string) string {
	resourceType, resourceID := ExtractResourceFromPath(path)
	if resourceType == "" {
		return strings.ToLower(method) + ".unknown"
	}

	// Check for sub-action (e.g. /start, /stop after the resource ID).
	parts := splitPath(path)
	subAction := extractSubAction(parts, resourceID)
	if subAction != "" {
		return resourceType + "." + subAction
	}

	verb := methodToVerb(method, resourceID)
	return resourceType + "." + verb
}

// ─── internal helpers ────────────────────────────────────────────────────────

// isReadOnlyMethod returns true for HTTP methods that do not mutate state.
func isReadOnlyMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

// resolveActorID returns the most appropriate actor identifier from the context.
func resolveActorID(c *gin.Context) string {
	if id := GetUserID(c); id != "" {
		return id
	}
	if v, exists := c.Get(apiKeyIDContextKey); exists {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// resolveActorType returns the actor type string for the audit entry.
func resolveActorType(c *gin.Context) string {
	if v, exists := c.Get(actorTypeContextKey); exists {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return GetUserType(c)
}

// splitPath splits a URL path into non-empty segments.
func splitPath(path string) []string {
	raw := strings.Split(path, "/")
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// resourceNouns maps plural path segments to their singular resource name.
var resourceNouns = map[string]string{
	"vms":          "vm",
	"nodes":        "node",
	"customers":    "customer",
	"users":        "user",
	"networks":     "network",
	"volumes":      "volume",
	"snapshots":    "snapshot",
	"templates":    "template",
	"api-keys":     "api_key",
	"audit-logs":   "audit_log",
	"roles":        "role",
	"permissions":  "permission",
	"provisioning": "provisioning",
}

// resourceNoun resolves a path segment to a singular resource noun.
func resourceNoun(segment string) (string, bool) {
	noun, ok := resourceNouns[strings.ToLower(segment)]
	return noun, ok
}

// uuidRegex matches UUID v4 strings.
var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// isIdentifier returns true if the segment looks like a resource ID (UUID or
// opaque alphanumeric identifier, not a sub-action keyword).
func isIdentifier(segment string) bool {
	if uuidRegex.MatchString(segment) {
		return true
	}
	// A non-UUID opaque ID is alphanumeric without hyphens in the middle;
	// sub-action keywords are typically lower-case English words.
	// We conservatively treat segments with digits as IDs.
	for _, r := range segment {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

// extractSubAction finds any action keyword that follows the resource ID in the path.
func extractSubAction(parts []string, resourceID string) string {
	if resourceID == "" {
		return ""
	}

	for i, p := range parts {
		if p == resourceID && i+1 < len(parts) {
			return strings.ToLower(parts[i+1])
		}
	}

	return ""
}

// methodToVerb maps an HTTP method to a CRUD verb, taking the presence of a
// resource ID into account for POST (create vs. action on existing resource).
func methodToVerb(method, resourceID string) string {
	switch strings.ToUpper(method) {
	case http.MethodPost:
		if resourceID != "" {
			return "action"
		}
		return "create"
	case http.MethodPut:
		return "replace"
	case http.MethodPatch:
		return "update"
	case http.MethodDelete:
		return "delete"
	default:
		return strings.ToLower(method)
	}
}
