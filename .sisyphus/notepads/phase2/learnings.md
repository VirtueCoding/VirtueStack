# Phase 2 Learnings

## Session: ses_32782a8a3ffe50jpztvXn3PBxL (2026-03-10)

### Patterns Established
- Repository pattern uses generic `ScanRow[T any]()` and `ScanRows[T any]()` helpers
- All DB queries parameterized with `$1, $2, ...` placeholders
- `pgx.ErrNoRows` always maps to `shared/errors.ErrNotFound`
- List operations return `(items, totalCount, error)` for pagination
- Sensitive fields use `json:"-"` tags to prevent accidental exposure

### Conventions
- All structs embed `base.Timestamps` and `base.SoftDelete`
- UUIDs stored as `string` (pgx handles UUID type automatically)
- All repository methods accept `context.Context` as first parameter
- Error handling: return errors up, let caller decide handling
- Logging: use `slog` with correlation ID from context

### File Structure
- Models: `internal/controller/models/*.go` — pure data structures
- Repository: `internal/controller/repository/*.go` — DB operations only
- Services: `internal/controller/services/*.go` — business logic, orchestration
- API: `internal/controller/api/{provisioning,customer,admin}/*.go` — HTTP handlers
- Tasks: `internal/controller/tasks/handlers.go` — async task implementations

---

## Session: customer_repo creation (2026-03-11)

### Repository Pattern Applied
- CustomerRepository: 9 core methods (Create, GetByID, GetByEmail, List, UpdateStatus, UpdateWHMCSClientID, SoftDelete) + 6 session methods
- AdminRepository: 7 methods (Create, GetByID, GetByEmail, List, UpdateTOTPEnabled, UpdatePasswordHash)
- Session methods live in CustomerRepository for simplicity (shared database operations)
- Filter types (CustomerListFilter, AdminListFilter) at bottom of file

### SQL Patterns
- Soft delete uses `status = 'deleted'` not a deleted_at column (customers table)
- Search filter uses `ILIKE` for case-insensitive matching on email/name
- All updates check `status != 'deleted'` to avoid modifying soft-deleted records
- Session table uses separate user_id + user_type columns for polymorphic user references

---

## Session: ip_repo creation (2026-03-11)

### IPRepository Methods (21 total)
- IPSet Operations (7): CreateIPSet, GetIPSetByID, GetIPSetByName, ListIPSets, UpdateIPSet, DeleteIPSet
- IPAddress Operations (8): CreateIPAddress, GetIPAddressByID, GetIPAddressByAddress, ListIPAddresses, AllocateIPv4, ReleaseIPv4, SetRDNS, GetRDNS
- IPv6Prefix Operations (3): CreateIPv6Prefix, GetIPv6PrefixByNode, DeleteIPv6Prefix
- VMIPv6Subnet Operations (3): CreateVMIPv6Subnet, GetVMIPv6SubnetsByVM, DeleteVMIPv6SubnetsByVM

### Atomic IP Allocation Pattern
- `AllocateIPv4`: Uses transaction with `SELECT FOR UPDATE SKIP LOCKED` to prevent race conditions
- Finds available IP, locks it, updates status to 'assigned', commits
- Returns error if no available addresses (not ErrNotFound - more descriptive)
- `ReleaseIPv4`: Uses transaction, sets status='cooldown', clears vm_id/customer_id, sets cooldown_until
- Both use `defer tx.Rollback(ctx)` pattern for safety

### SQL Column Selection Pattern
- Each entity has a `const xxxSelectCols` with all column names
- Scanner functions (`scanIPSet`, `scanIPAddress`, etc.) scan rows into structs
- Column order in SELECT must match Scan() parameter order exactly

### Filter Types
- `IPSetListFilter`: LocationID, IPVersion filters
- `IPAddressListFilter`: IPSetID, VMID, CustomerID, Status filters
- Both embed `models.PaginationParams` for Limit/Offset

---

## Session: ip_repo creation (2026-03-11)

### IPRepository Methods (21 total)
- IPSet Operations (7): CreateIPSet, GetIPSetByID, GetIPSetByName, ListIPSets, UpdateIPSet, DeleteIPSet
- IPAddress Operations (8): CreateIPAddress, GetIPAddressByID, GetIPAddressByAddress, ListIPAddresses, AllocateIPv4, ReleaseIPv4, SetRDNS, GetRDNS
- IPv6Prefix Operations (3): CreateIPv6Prefix, GetIPv6PrefixByNode, DeleteIPv6Prefix
- VMIPv6Subnet Operations (3): CreateVMIPv6Subnet, GetVMIPv6SubnetsByVM, DeleteVMIPv6SubnetsByVM

### Atomic IP Allocation Pattern
- `AllocateIPv4`: Uses transaction with `SELECT FOR UPDATE SKIP LOCKED` to prevent race conditions
- Finds available IP, locks it, updates status to 'assigned', commits
- Returns error if no available addresses (not ErrNotFound - more descriptive)
- `ReleaseIPv4`: Uses transaction, sets status='cooldown', clears vm_id/customer_id, sets cooldown_until
- Both use `defer tx.Rollback(ctx)` pattern for safety

### SQL Column Selection Pattern
- Each entity has a `const xxxSelectCols` with all column names
- Scanner functions (`scanIPSet`, `scanIPAddress`, etc.) scan rows into structs
- Column order in SELECT must match Scan() parameter order exactly

### Filter Types
- `IPSetListFilter`: LocationID, IPVersion filters
- `IPAddressListFilter`: IPSetID, VMID, CustomerID, Status filters
- Both embed `models.PaginationParams` for Limit/Offset

---


---

## Session: audit_repo creation (2026-03-11)

### AuditRepository Methods (8 total - APPEND-ONLY)
- Core Operations: Append, GetByID, List
- Filter Operations: ListByActor, ListByResource, ListByCorrelationID, ListRecent
- Maintenance: GetPartitionStats

### Critical Design Principle: Append-Only
- NO UPDATE or DELETE operations on audit_logs
- Database enforces this with `REVOKE UPDATE, DELETE ON audit_logs FROM app_user`
- Append method uses INSERT only, no RETURNING clause needed
- This ensures audit trail integrity for compliance

### Partitioned Table Pattern
- audit_logs is partitioned by timestamp (monthly partitions)
- Indexes: actor_id, action, resource (type+id), timestamp, correlation_id
- GetPartitionStats queries pg_class/pg_inherits for partition monitoring

### Filter Types
- `AuditLogFilter`: ActorID, ActorType, Action, ResourceType, ResourceID, Success, StartTime, EndTime
- `AuditPartitionStats`: PartitionName, RowCount, SizeBytes
- Both embed `models.PaginationParams` for Limit/Offset

### Immutable Records Pattern
- No soft delete - audit logs are permanent
- No UpdateStatus, UpdateX, or Delete methods
- List operations ordered by timestamp DESC (most recent first)
- ListByCorrelationID ordered ASC for request flow tracing
---

## Session: audit_repo creation (2026-03-11)

### AuditRepository Methods (8 total - APPEND-ONLY)
- Core Operations: Append, GetByID, List
- Filter Operations: ListByActor, ListByResource, ListByCorrelationID, ListRecent
- Maintenance: GetPartitionStats

### Critical Design Principle: Append-Only
- NO UPDATE or DELETE operations on audit_logs
- Database enforces this with `REVOKE UPDATE, DELETE ON audit_logs FROM app_user`
- Append method uses INSERT only, no RETURNING clause needed
- This ensures audit trail integrity for compliance

### Partitioned Table Pattern
- audit_logs is partitioned by timestamp (monthly partitions)
- Indexes: actor_id, action, resource (type+id), timestamp, correlation_id
- GetPartitionStats queries pg_class/pg_inherits for partition monitoring

### Filter Types
- `AuditLogFilter`: ActorID, ActorType, Action, ResourceType, ResourceID, Success, StartTime, EndTime
- `AuditPartitionStats`: PartitionName, RowCount, SizeBytes
- Both embed `models.PaginationParams` for Limit/Offset

### Immutable Records Pattern
- No soft delete - audit logs are permanent
- No UpdateStatus, UpdateX, or Delete methods
- List operations ordered by timestamp DESC (most recent first)
- ListByCorrelationID ordered ASC for request flow tracing
