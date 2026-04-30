---
applyTo: "proto/**/*.proto"
---

# Protocol Buffer Instructions

## Syntax and Package

Use `proto3` syntax. Package naming follows `virtuestack.{component}` (e.g., `virtuestack.nodeagent`). Set `go_package` to the full Go import path.

## Naming Conventions

- **Messages:** PascalCase (e.g., `CreateVMRequest`, `VMStatusResponse`).
- **Fields:** snake_case (e.g., `vm_id`, `disk_size_gb`).
- **Enums:** UPPER_SNAKE_CASE, prefixed with the enum type name (e.g., `VM_STATUS_RUNNING`).
- **Services:** PascalCase with `Service` suffix (e.g., `NodeAgentService`).
- **RPC methods:** PascalCase verb + noun (e.g., `CreateVM`, `GetVMStatus`).

## Enum Rules

- First enum value must always be `UNKNOWN = 0` (the protobuf default).
- Prefix all enum values with the enum name (e.g., `STORAGE_TYPE_CEPH`, `STORAGE_TYPE_QCOW`).

## Documentation

- Document every field, enum value, and RPC method with comments explaining purpose.
- Use section headers (e.g., `// === ENUMERATIONS ===`) to organize large files.

## Temporal Fields

Use `google.protobuf.Timestamp` for all date/time fields — never strings or integers.

## After Changes

Regenerate Go code with `make proto` after modifying `.proto` files.
