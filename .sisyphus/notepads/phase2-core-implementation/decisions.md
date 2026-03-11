# Phase 2 Core Implementation Architectural Decisions

## VM Service Layer (2026-03-11)

### Decision: Interface-based Dependencies
**Rationale:** Using interfaces (`TaskPublisher`, `NodeAgentClient`, `IPAMService`) instead of concrete types allows:
- Easy testing with mocks
- Decoupling from NATS implementation details
- Decoupling from generated protobuf code
- Future flexibility for different implementations

**Trade-offs:**
- Slightly more verbose than using concrete types
- Need to maintain interface definitions

### Decision: Async vs Sync Operations
**Rationale:**
- Async operations (create, delete, reinstall) involve long-running tasks (10-300s) that would block HTTP requests
- Publishing to task queue allows immediate response with task_id for polling
- Synchronous operations (start, stop, restart) are quick (~5s) and provide immediate feedback

**Trade-offs:**
- Async operations require client polling or WebSocket notifications
- Synchronous operations may timeout if node agent is slow

### Decision: Soft Delete for VMs
**Rationale:**
- Preserves VM records for billing, auditing, and potential recovery
- Allows async cleanup tasks to run after API response
- Consistent with data retention best practices

**Trade-offs:**
- Requires filtering deleted records in all queries
- Uses more storage space

### Decision: Node Selection Algorithm
**Rationale:**
- Use `GetLeastLoadedNode()` to distribute VMs evenly across nodes
- Fall back to any online node if no location specified
- Pick node with most available memory as tiebreaker

**Trade-offs:**
- Doesn't consider node health beyond online status
- Doesn't consider network topology or storage locality

### Decision: Password Encryption in Service Layer
**Rationale:**
- Passwords are encrypted using AES-256-GCM before storage
- Encryption happens in service layer (not repository) to keep crypto logic centralized
- Service has access to encryption key via dependency injection

**Trade-offs:**
- Service layer has additional responsibility
- Need to manage encryption key lifecycle