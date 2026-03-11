# Phase 3 Networking Security - Learnings

## 2026-03-11: nwfilter.go Implementation

### libvirt nwfilter API
- `conn.NWFilterDefineXML(xmlStr)` - Define a new network filter
- `conn.LookupNWFilterByName(name)` - Look up a filter by name
- `conn.ListAllNWFilters(0)` - List all nwfilters
- `filter.Undefine()` - Remove a filter
- `filter.Free()` - Free the filter reference after use
- `filter.GetXMLDesc(0)` - Get the XML definition of a filter
- `filter.GetUUIDString()` - Get the filter's UUID
- `filter.GetName()` - Get the filter's name

### nwfilter XML Structure
- nwfilter uses XML with `<filter>` root element
- Rules use `<rule action='accept|drop' direction='in|out|inout' priority='N'>`
- Protocol elements: `<mac>`, `<ip>`, `<ipv6>`, `<arp>`, `<all>`
- Key attributes:
  - `srcmacaddr` - Source MAC address
  - `srcipaddr` - Source IP address
  - `arpsrcmacaddr` - ARP source MAC
  - `arpsrcipaddr` - ARP source IP
  - `match='yes'` - Enable matching

### Error Handling
- `libvirt.ERR_NO_NWFILTER` - Filter not found error code
- Use `err.(libvirt.Error)` type assertion to access error details
- Always call `filter.Free()` after use to prevent resource leaks

### Naming Conventions
- Filter names use prefix `vs-anti-spoof-` for VirtueStack filters
- Base filter `clean-traffic` provides common protections
- Filter names must be valid XML attribute values

### Anti-Spoofing Rules Priority
- ARP rules: priority 50 (highest priority)
- IPv4 rules: priority 100+
- IPv6 rules: priority 200+
- MAC rules: priority 500
- Drop rules: priority 1000 (lowest priority)

### Integration Pattern
1. Create filter with `NWFilterDefineXML` before VM start
2. Reference filter in domain XML via `<filterref filter='vs-anti-spoof-vmname'>`
3. Remove filter with `Undefine()` when VM is deleted

## Phase 3 Analysis Learnings (2026-03-11)

### Networking Architecture (L2 vs L3)
- Live Migration requires special consideration for IP routing.
- **Flaw discovered**: Tying IPv6 /48 prefixes directly to physical nodes breaks SLAAC routing when a VM migrates.
- **Pattern adopted**: IP Sets (/48s) belong to the Location/VLAN, not the Node. Node agents should NOT run local `radvd` instances if VMs are expected to live-migrate across an L2 bridge. The upstream router handles RA/SLAAC.

### Security: libvirt nwfilter
- **Critical Trap**: `nwfilter` rules are executed by priority (lowest number first). If a base filter (e.g. `clean-traffic`) contains an `accept all` rule at a very low priority (e.g. `-500`), it completely bypasses all subsequent IP/ARP anti-spoofing rules.
- **DHCP Trap**: To prevent rogue DHCP servers, you must explicitly drop outbound UDP port 67 traffic from VMs. To allow legitimate DHCP clients, you must explicitly allow outbound UDP 68 -> 67 from `0.0.0.0`.

### Distributed Systems: Rate Limiting
- **Race Condition**: A typical Redis sliding window rate limiter (`ZREMRANGEBYSCORE`, `ZCARD`, `ZADD`) must be executed as an atomic Lua script (`Eval`). Doing it sequentially in Go will result in race conditions allowing bursts to bypass the limit under high concurrency.

### Security: Command Injection
- **Validation**: Even when using `exec.CommandContext` (which prevents shell injection), you must strictly validate variables passed to commands like `tc` or `ip`. An interface name like `-h` could be treated as a flag. Always use strict regex validation like `^[a-zA-Z0-9_]+$`.

### HA Failover (Split-Brain)
- **CRITICAL**: Never release a Ceph RBD exclusive lock during HA failover without first fencing the dead node. Even if an admin manually confirms it's down, you must mandate `ceph osd blocklist add` before lock release to prevent split-brain data corruption if the node comes back online.

### Async Tasks & Live Migration
- **State Loss**: Traffic shaping rules (`tc`) applied to local `vnet` tap interfaces are destroyed during libvirt live migration. The Controller must call a `PostMigrateSetup` gRPC hook on the destination node to re-apply them.
- **Idempotency**: NATS JetStream retries tasks on failure. Task handlers must check current state (e.g., `if image_exists()`) before executing actions that aren't inherently idempotent (like `rbd clone`).