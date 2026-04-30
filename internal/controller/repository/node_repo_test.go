package repository

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubNodeRow struct {
	values []any
}

func (r stubNodeRow) Scan(dest ...any) error {
	for i, value := range r.values {
		switch target := dest[i].(type) {
		case *string:
			if value == nil {
				return fmt.Errorf("can't scan into dest[%d]: cannot scan NULL into *string", i)
			}
			*target = value.(string)
		case **string:
			if value == nil {
				*target = nil
				continue
			}
			val := value.(string)
			*target = &val
		case *int:
			*target = value.(int)
		case *time.Time:
			*target = value.(time.Time)
		case **time.Time:
			if value == nil {
				*target = nil
				continue
			}
			val := value.(time.Time)
			*target = &val
		case *sql.NullString:
			if value == nil {
				*target = sql.NullString{}
				continue
			}
			*target = sql.NullString{String: value.(string), Valid: true}
		default:
			return fmt.Errorf("unsupported destination type %T", target)
		}
	}

	return nil
}

func TestScanNodeHandlesNullableCephPool(t *testing.T) {
	now := time.Date(2026, time.April, 7, 8, 30, 0, 0, time.UTC)
	locationID := "99999999-9999-9999-9999-999999999001"
	ipmiAddress := "10.0.0.10"

	tests := []struct {
		name         string
		cephPool     any
		wantCephPool string
	}{
		{
			name:         "null ceph pool defaults empty string",
			cephPool:     nil,
			wantCephPool: "",
		},
		{
			name:         "non-null ceph pool preserved",
			cephPool:     "vs-vms",
			wantCephPool: "vs-vms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := scanNode(stubNodeRow{
				values: []any{
					"node-1",
					"node-1.example.test",
					"10.0.0.2:50051",
					"10.0.0.2",
					locationID,
					"online",
					32,
					65536,
					8,
					16384,
					tt.cephPool,
					ipmiAddress,
					nil,
					nil,
					now,
					0,
					now,
					"ceph",
					"",
					nil,
					nil,
				},
			})

			require.NoError(t, err)
			assert.Equal(t, tt.wantCephPool, node.CephPool)
			require.NotNil(t, node.LocationID)
			assert.Equal(t, locationID, *node.LocationID)
			require.NotNil(t, node.IPMIAddress)
			assert.Equal(t, ipmiAddress, *node.IPMIAddress)
		})
	}
}
