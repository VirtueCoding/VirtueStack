# Phase 2 Core Implementation Learnings

## Auth Service Implementation (2026-03-11)

### Argon2id Password Hashing
- Use `github.com/alexedwards/argon2id` package (already in go.mod)
- Recommended parameters: Memory=64MB, Iterations=3, Parallelism=4, SaltLength=16, KeyLength=32
- Package provides both `CreateHash()` and `ComparePasswordAndHash()` functions

### Token Generation
- Reuse existing middleware functions from `internal/controller/api/middleware/auth.go`:
  - `GenerateAccessToken()` - JWT with HMAC-SHA256
  - `GenerateRefreshToken()` - 64-char hex random token
  - `GenerateTempToken()` - Short-lived JWT for 2FA flow
  - `ValidateTempToken()` - Validates and extracts claims from temp token

### TOTP Verification
- Use `github.com/pquerna/otp/totp` package
- TOTP secrets are stored encrypted in database using AES-256-GCM
- Use `crypto.Decrypt()` to decrypt before verification
- ValidateCustom with Skew=1 allows ±30 seconds tolerance

### Session Management
- Sessions stored in `sessions` table via CustomerRepository
- Refresh tokens are SHA-256 hashed before storage
- Session lookup by `refresh_token_hash` for refresh flow
- Token rotation: delete old session, create new session with new refresh token

### Auth Flow Patterns
- Customer login: 2FA optional (if enabled, return temp_token)
- Admin login: 2FA mandatory (always return temp_token)
- Login methods return `(*models.AuthTokens, string, error)` where second string is refresh_token

### Security Considerations
- Never log passwords or tokens
- Use constant-time comparison for sensitive values
- Invalidate all sessions after password change
- Don't reveal whether email exists in password reset

## VM Service Implementation (2026-03-11)

### Service Layer Architecture
- Services call repositories and abstract gRPC clients (not direct DB or libvirt)
- Async operations (create, delete, reinstall) publish tasks, don't execute directly
- Synchronous operations (start, stop, restart) call node agent gRPC directly
- All methods accept `context.Context` first
- RBAC: always verify VM ownership before operations, admins bypass ownership check

### Interface Abstractions
- `TaskPublisher` - abstracts NATS task publishing for async operations
- `NodeAgentClient` - abstracts gRPC calls to node agents (decouples from generated proto)
- `IPAMService` - abstracts IP allocation/release operations
- `DefaultTaskPublisher` - basic implementation that writes to database (for dev/testing)

### VM Lifecycle Patterns
- `CreateVM`: Validate plan/template → find node → allocate IPs → create DB record → publish task
- `StartVM/StopVM/RestartVM`: Verify ownership → check status → call gRPC → update DB
- `DeleteVM`: Verify ownership → publish task → soft delete record → release IPs
- `ReinstallVM`: Verify ownership → validate template → publish task
- `ResizeVM`: Verify ownership → validate resources → call gRPC → update DB

### Key Dependencies
- `repository.VMRepository` - VM CRUD operations
- `repository.NodeRepository` - Node operations including `GetLeastLoadedNode()`
- `repository.IPRepository` - IP allocation/release with `AllocateIPv4()`, `ReleaseIPv4()`
- `repository.PlanRepository` - Plan validation
- `repository.TemplateRepository` - Template validation
- `repository.TaskRepository` - Task status tracking
- `crypto.Encrypt()` for password encryption
- `crypto.GenerateMACAddress()` for VM network interfaces

### Models Added
- `models.VMMetrics` - Real-time resource utilization (CPU, memory, disk, network)
- `models.NodeListFilter` - Filter for listing nodes (was referenced but missing)

### libvirt Domain Naming
- Format: `vm-{hostname}-{short-uuid}` where short-uuid is first 8 chars of VM UUID
- Example: `vm-webserver-a1b2c3d4`