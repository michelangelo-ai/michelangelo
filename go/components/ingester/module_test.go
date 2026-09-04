package ingester

import (
	"testing"

	"github.com/michelangelo-ai/michelangelo/go/storage"
	"github.com/stretchr/testify/assert"
)

// TestIsMySQLPrimaryKind covers the gate register() uses to skip setting up an ingester
// reconciler entirely for a MySQL-primary kind (see storage.MySQLPrimaryPolicy): such a kind is
// never written to etcd, so there is nothing for the ingester to reconcile or evict.
func TestIsMySQLPrimaryKind(t *testing.T) {
	tests := []struct {
		name   string
		policy storage.MySQLPrimaryPolicy
		kind   string
		want   bool
	}{
		{
			name:   "nil policy opts nothing in",
			policy: nil,
			kind:   "Metric",
			want:   false,
		},
		{
			name:   "kind not in the opted-in set",
			policy: storage.NewStaticMySQLPrimaryPolicy("Metric"),
			kind:   "RayJob",
			want:   false,
		},
		{
			name:   "kind opted in",
			policy: storage.NewStaticMySQLPrimaryPolicy("Metric"),
			kind:   "Metric",
			want:   true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isMySQLPrimaryKind(tt.policy, tt.kind))
		})
	}
}
