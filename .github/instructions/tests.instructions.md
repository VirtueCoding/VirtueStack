---
applyTo: "**/*_test.go"
---

# Go Test Instructions

## Test Structure

Always use table-driven tests with subtests:

```go
tests := []struct {
    name    string
    input   InputType
    want    OutputType
    wantErr bool
}{
    {"valid input", validInput, expectedOutput, false},
    {"empty input", emptyInput, zero, true},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        got, err := functionUnderTest(tt.input)
        if tt.wantErr {
            require.Error(t, err)
            return
        }
        require.NoError(t, err)
        assert.Equal(t, tt.want, got)
    })
}
```

## Required Test Cases

Every test suite must cover: happy path, invalid input (missing/wrong types/boundary values), edge cases, authorization scenarios, and error paths.

## Assertions

Use `testify` — `require` for fatal checks (stops test on failure), `assert` for non-fatal checks.

## Mocking

Mock interfaces using function fields:

```go
type mockDB struct {
    queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}
```

## Running Tests

```bash
make test       # Controller/shared unit tests only (no native libs needed)
make test-race  # Same with race detector (CI runs this)
go test -race -run TestFunctionName ./internal/controller/services/...  # Single test
```

Node Agent tests require libvirt/Ceph dev headers: `make test-native`.

## Prohibitions

- No tests without assertions.
- No execution-order dependencies between tests.
- No `time.Sleep` — use channels, timers, or test helpers.
- No shared mutable state between tests.
- No testing of implementation details (internal state) — test behavior.
