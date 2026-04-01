---
applyTo: "**/*.go"
---

# Go Code Instructions

## Architecture

Follow the layered pattern: Handler (`api/{tier}/`) → Service (`services/`) → Repository (`repository/`) → PostgreSQL. Never call repositories directly from handlers.

Services use a config struct for dependency injection:

```go
type VMServiceConfig struct {
    VMRepo        *repository.VMRepository
    Logger        *slog.Logger
}
func NewVMService(cfg VMServiceConfig) *VMService { ... }
```

## Error Handling

- Handle every error; never use `_` to discard errors without a comment.
- Wrap errors with context: `fmt.Errorf("create vm: %w", err)`.
- Use `errors.Is()` and `errors.As()` — never compare error strings.
- Use sentinel errors from `internal/shared/errors/errors.go` (`ErrNotFound`, `ErrUnauthorized`, `ErrValidation`, `ErrConflict`).
- In HTTP handlers, always use `middleware.RespondWithError(c, status, code, message)` — never `c.JSON()` for errors.

## Validation

- Use `validate` struct tags with `middleware.BindAndValidate(c, &req)` in handlers.
- Use `go-playground/validator` for struct validation.

## Logging

- Use `slog.Logger` with component context: `logger.With("component", "service-name")`.
- Include `correlation_id` in error logs: `middleware.GetCorrelationID(c)`.
- Never log passwords, tokens, API keys, or PII.

## Concurrency

- Always pass `ctx context.Context` as the first parameter to service/repository methods.
- Every goroutine must have cancellation via `context.Context`.
- Track all goroutines with `sync.WaitGroup` or `errgroup.Group`.
- Use directional channel types (`chan<-`, `<-chan`).

## Naming

- Allowed abbreviations: `id`, `url`, `http`, `ctx`, `err`, `req`, `res`, `db`, `tx`, `ip`, `vm`, `os`, `io`, `rpc`.
- Functions: verb + noun (e.g., `CreateVM`, `ValidateInput`).
- Booleans: prefix with `is`, `has`, `can`, `should`.
- Mark sensitive model fields with `json:"-"`.

## Linting

Run `make lint` (golangci-lint with 24 linters). Key enforced rules:
- `errcheck`: all errors handled
- `gosec`: security issues detected
- `contextcheck`: context.Context as first param
- `bodyclose`: HTTP response bodies closed
- `sqlclosecheck`: SQL connections closed
- `revive`: exported names documented, error naming conventions

## Prohibitions

- No `fmt.Println` or `log.Print` — use `slog`.
- No `math/rand` for security — use `crypto/rand`.
- No bare `interface{}` — use specific types or generics.
- No commented-out code.
- No `TODO`/`FIXME`/`HACK` comments.
- No functions longer than 40 lines or nesting deeper than 3 levels.
- No functions with more than 4 parameters — use options struct.
