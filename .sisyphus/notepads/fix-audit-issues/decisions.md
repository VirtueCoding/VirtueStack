# Phase 2.1: Node Failover Architectural Decisions

## 2026-03-11

### Decision: Use ipmitool Command-Line Tool for IPMI

**Rationale:**
- Native Go IPMI libraries (like `github.com/vmware/goipmi`) have complex dependencies
- `ipmitool` is the industry standard, widely tested, and available in most distributions
- Simpler to maintain and debug
- Avoids potential library licensing issues

**Trade-offs:**
- Requires `ipmitool` installed on controller host
- Slightly slower than native Go implementation due to process spawn overhead
- Password visible in process list briefly (mitigated by quick execution)

### Decision: Circuit Breaker In-Memory Storage

**Rationale:**
- Failover state is transient and node-specific
- In-memory map with mutex is sufficient for single controller instance
- Simpler than persisting to database

**Trade-offs:**
- State lost on controller restart (acceptable since failover is rare)
- Does not work for multi-controller HA setup (future: use Redis or distributed state)

### Decision: Notification Graceful Degradation

**Rationale:**
- Notification failures should not block failover process
- Partial success (some channels work) is better than total failure
- All failures logged for debugging

**Trade-offs:**
- May miss alerts if all channels fail silently
- Requires monitoring of notification logs

### Decision: VM Migration via EvacuateNode gRPC Call

**Rationale:**
- Reuses existing `NodeAgentClient.EvacuateNode()` interface
- Node agent handles the complexity of live migration
- Controller doesn't need to know libvirt details

**Trade-offs:**
- All-or-nothing approach - no partial migration tracking
- Depends on node agent being reachable (may not be during failure)