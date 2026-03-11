# Phase 2 Decisions

## Session: ses_32782a8a3ffe50jpztvXn3PBxL (2026-03-10)

### Architecture Choices
1. **Service layer separation:** Business logic in `services/`, not in handlers — enables testing and reuse
2. **Task handlers separate:** Async operations in `tasks/handlers.go`, not in services — clear sync vs async boundary
3. **API tier separation:** Three distinct route groups (provisioning, customer, admin) — different auth requirements

### Technology Choices
1. **JWT with HMAC-SHA256:** Sufficient for internal auth, no need for RSA complexity
2. **Argon2id for passwords:** Per MASTER_CODING_STANDARD_V2.md requirement
3. **NATS JetStream for tasks:** Durable consumers, ack-based processing, retry logic

---
