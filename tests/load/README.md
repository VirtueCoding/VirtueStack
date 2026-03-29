# VirtueStack Load Testing (k6)

This directory contains k6 scripts for exercising high-traffic API paths.

## Prerequisites

- Install k6: <https://k6.io/docs/get-started/installation/>
- Start a reachable VirtueStack environment (Controller + required dependencies)
- Export the required auth/runtime variables for the script you run

## Scripts

### 1) VM operations baseline

```bash
k6 run /home/runner/work/VirtueStack/VirtueStack/tests/load/k6-vm-operations.js
```

Environment variables (typical):

- `BASE_URL` (default: `http://localhost:8080`)
- `CUSTOMER_TOKEN`
- `ADMIN_TOKEN` (optional, used for cleanup)
- `TEST_VM_ID`

### 2) Provisioning create concurrency

```bash
k6 run /home/runner/work/VirtueStack/VirtueStack/tests/load/k6-provisioning-create.js
```

Required:

- `PROVISIONING_API_KEY`

Optional:

- `BASE_URL`, `CUSTOMER_ID`, `PLAN_ID`, `TEMPLATE_ID`, `LOCATION_ID`

### 3) Customer list pagination/filter pressure

```bash
k6 run /home/runner/work/VirtueStack/VirtueStack/tests/load/k6-customer-list.js
```

Required:

- `CUSTOMER_TOKEN`

Optional:

- `BASE_URL`

### 4) Power operations concurrency

```bash
k6 run /home/runner/work/VirtueStack/VirtueStack/tests/load/k6-power-operations.js
```

Required:

- `PROVISIONING_API_KEY`
- `TEST_VM_ID`

Optional:

- `BASE_URL`

### 5) Admin listing with filters/pagination

```bash
k6 run /home/runner/work/VirtueStack/VirtueStack/tests/load/k6-admin-listing.js
```

Required:

- `ADMIN_TOKEN`

Optional:

- `BASE_URL`

### 6) Task throughput (create + poll)

```bash
k6 run /home/runner/work/VirtueStack/VirtueStack/tests/load/k6-task-throughput.js
```

Required:

- `PROVISIONING_API_KEY`
- `TEST_VM_ID`

Optional:

- `BASE_URL`

## Run all load scripts

From repository root:

```bash
make load-test
```

This target runs all scripts in sequence:

1. `k6-vm-operations.js`
2. `k6-provisioning-create.js`
3. `k6-customer-list.js`
4. `k6-power-operations.js`
5. `k6-admin-listing.js`
6. `k6-task-throughput.js`
