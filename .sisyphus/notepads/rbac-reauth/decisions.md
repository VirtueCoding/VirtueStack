# RBAC Re-auth Implementation Decisions

## Decision: Use Session-based Re-auth Tracking

**Rationale**: Tracking re-authentication at the session level (not user level) provides:
- Granularity: Different sessions can have different re-auth states
- Security: Closing a session invalidates the re-auth
- Flexibility: Supports multi-device scenarios correctly

## Decision: 5-Minute Re-auth Window

**Rationale**: 
- Industry standard for sensitive operations
- Balances security (prevents stale auth) with UX (not too frequent)
- Defined as constant `ReAuthWindow = 5 * time.Minute` for easy adjustment

## Decision: Fail-Safe on Errors

**Rationale**: 
- If session lookup fails, require re-auth
- Prevents accidental authorization on database errors
- Security principle: deny by default when uncertain

## Decision: Pointer Type for LastReauthAt

**Rationale**:
- NULL in database represents "never re-authenticated"
- Pointer type naturally maps to SQL NULL
- Clear semantic: nil = no re-auth recorded

## Decision: Include CustomerRepo in RBACService

**Rationale**:
- RBAC needs to query session data for re-auth checks
- CustomerRepo already has session management methods
- Avoids creating a separate SessionRepository
- Keeps dependency graph simple

## Decision: Update All Session Queries

**Rationale**:
- Consistency: All session queries return complete Session struct
- Future-proof: New code can rely on LastReauthAt being populated
- Performance: Single query gets all session data
