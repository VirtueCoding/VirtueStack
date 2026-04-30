package repository

import (
	"context"
	"fmt"
	"net/netip"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIPRepository_ListIPAddressesBuildsScanableSelect(t *testing.T) {
	t.Parallel()

	var gotSQL string
	mock := &mockDB{
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			gotSQL = sql
			return &emptyRows{}, nil
		},
	}

	vmID := "99999999-9999-9999-9999-999999999001"
	repo := NewIPRepository(mock)
	_, _, _, err := repo.ListIPAddresses(context.Background(), IPAddressListFilter{
		PaginationParams: models.PaginationParams{PerPage: 20},
		VMID:             &vmID,
	})

	require.NoError(t, err)
	require.NotEmpty(t, gotSQL)
	assert.Regexp(t, regexp.MustCompile(`(?is)^select\s+id,\s*ip_set_id`), gotSQL)
	assert.NotRegexp(t, regexp.MustCompile(`(?is)^select\s+1\s*,`), gotSQL)
}

func TestIPRepository_ListIPSetsBuildsScanableSelect(t *testing.T) {
	t.Parallel()

	var gotSQL string
	mock := &mockDB{
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			gotSQL = sql
			return &emptyRows{}, nil
		},
	}

	repo := NewIPRepository(mock)
	_, _, _, err := repo.ListIPSets(context.Background(), IPSetListFilter{
		PaginationParams: models.PaginationParams{PerPage: 20},
	})

	require.NoError(t, err)
	require.NotEmpty(t, gotSQL)
	assert.Regexp(t, regexp.MustCompile(`(?is)^select\s+id,\s*name`), gotSQL)
	assert.NotRegexp(t, regexp.MustCompile(`(?is)^select\s+1\s*,`), gotSQL)
}

func TestIPRepository_ListIPSetsScansNetworkTypes(t *testing.T) {
	t.Parallel()

	locationID := "22222222-2222-2222-2222-222222222001"
	vlanID := 100
	createdAt := time.Date(2026, time.April, 7, 11, 10, 0, 0, time.UTC)

	mock := &mockDB{
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return &stubIPRows{
				rows: [][]any{{
					"44444444-4444-4444-4444-444444444001",
					"Test Public IPv4",
					&locationID,
					netip.MustParsePrefix("192.0.2.0/24"),
					netip.MustParseAddr("192.0.2.1"),
					&vlanID,
					int16(4),
					[]string{"33333333-3333-3333-3333-333333333001"},
					createdAt,
				}},
			}, nil
		},
	}

	repo := NewIPRepository(mock)
	ipSets, hasMore, lastID, err := repo.ListIPSets(context.Background(), IPSetListFilter{
		PaginationParams: models.PaginationParams{PerPage: 20},
	})

	require.NoError(t, err)
	require.Len(t, ipSets, 1)
	assert.False(t, hasMore)
	assert.Equal(t, "44444444-4444-4444-4444-444444444001", lastID)
	assert.Equal(t, "192.0.2.0/24", ipSets[0].Network)
	assert.Equal(t, "192.0.2.1", ipSets[0].Gateway)
	assert.Equal(t, []string{"33333333-3333-3333-3333-333333333001"}, ipSets[0].NodeIDs)
}

func TestIPRepository_ListIPAddressesDefaultsPerPage(t *testing.T) {
	t.Parallel()

	vmID := "99999999-9999-9999-9999-999999999001"
	customerID := "88888888-8888-8888-8888-888888888001"
	rdns := "test-vm-running.example.test"
	assignedAt := time.Date(2026, time.April, 7, 9, 30, 0, 0, time.UTC)
	createdAt := assignedAt

	mock := &mockDB{
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return &stubIPRows{
				rows: [][]any{{
					"55555555-5555-5555-5555-555555555001",
					"44444444-4444-4444-4444-444444444001",
					netip.MustParseAddr("192.0.2.10"),
					int16(4),
					&vmID,
					&customerID,
					true,
					&rdns,
					models.IPStatusAssigned,
					&assignedAt,
					nil,
					nil,
					createdAt,
				}},
			}, nil
		},
	}

	repo := NewIPRepository(mock)
	ips, hasMore, lastID, err := repo.ListIPAddresses(context.Background(), IPAddressListFilter{
		VMID: &vmID,
	})

	require.NoError(t, err)
	require.Len(t, ips, 1)
	assert.False(t, hasMore)
	assert.Equal(t, "55555555-5555-5555-5555-555555555001", lastID)
	assert.Equal(t, "192.0.2.10", ips[0].Address)
	assert.True(t, ips[0].IsPrimary)
}

type stubIPRows struct {
	rows [][]any
	idx  int
}

func (r *stubIPRows) Close() {}

func (r *stubIPRows) Err() error { return nil }

func (r *stubIPRows) CommandTag() pgconn.CommandTag { return pgconn.NewCommandTag("SELECT 1") }

func (r *stubIPRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (r *stubIPRows) Next() bool {
	if r.idx >= len(r.rows) {
		return false
	}
	r.idx++
	return true
}

func (r *stubIPRows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx > len(r.rows) {
		return fmt.Errorf("scan called before Next")
	}
	current := r.rows[r.idx-1]
	if len(dest) != len(current) {
		return fmt.Errorf("scan destination count mismatch: got %d want %d", len(dest), len(current))
	}
	for i := range dest {
		if err := assignIPTestValue(dest[i], current[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *stubIPRows) Values() ([]any, error) {
	if r.idx == 0 || r.idx > len(r.rows) {
		return nil, fmt.Errorf("values called before Next")
	}
	return r.rows[r.idx-1], nil
}

func (r *stubIPRows) RawValues() [][]byte { return nil }

func (r *stubIPRows) Conn() *pgx.Conn { return nil }

func assignIPTestValue(dest any, value any) error {
	dv := reflect.ValueOf(dest)
	if dv.Kind() != reflect.Ptr || dv.IsNil() {
		return fmt.Errorf("destination is not a pointer")
	}
	if value == nil {
		dv.Elem().Set(reflect.Zero(dv.Elem().Type()))
		return nil
	}

	v := reflect.ValueOf(value)
	if v.Type().AssignableTo(dv.Elem().Type()) {
		dv.Elem().Set(v)
		return nil
	}
	if v.Type().ConvertibleTo(dv.Elem().Type()) {
		dv.Elem().Set(v.Convert(dv.Elem().Type()))
		return nil
	}
	return fmt.Errorf("cannot assign %T to %T", value, dest)
}
