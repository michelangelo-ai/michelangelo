package storage

// MySQLPrimaryPolicy answers whether a kind's objects are created and updated directly in
// MetadataStorage rather than in k8s/ETCD. Unlike the ImmutableAnnotation eviction path — where an
// object lives in etcd until its spec is frozen, then moves to MetadataStorage read-only except for
// labels/annotations — a MySQL-primary kind never has an etcd copy at all: Create, Update,
// UpdateStatus and Delete all go straight to MetadataStorage, and the object's spec/status remain
// live-updatable for its entire lifetime.
//
// The set of MySQL-primary kinds is injected at the composition root, mirroring
// cascadedelete.RetainPolicy.
type MySQLPrimaryPolicy interface {
	// IsMySQLPrimary reports whether objects of the given Kind should be created and updated
	// directly in MetadataStorage, bypassing k8s/ETCD entirely.
	IsMySQLPrimary(kind string) bool
}

// staticMySQLPrimaryPolicy is a MySQLPrimaryPolicy backed by a fixed set of kinds.
type staticMySQLPrimaryPolicy struct {
	kinds map[string]bool
}

// NewStaticMySQLPrimaryPolicy returns a MySQLPrimaryPolicy that treats exactly the given kinds as
// MySQL-primary, supplied by the caller (the composition root).
func NewStaticMySQLPrimaryPolicy(kinds ...string) MySQLPrimaryPolicy {
	set := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		set[k] = true
	}
	return staticMySQLPrimaryPolicy{kinds: set}
}

// IsMySQLPrimary reports whether the given kind is in the MySQL-primary set.
func (p staticMySQLPrimaryPolicy) IsMySQLPrimary(kind string) bool {
	return p.kinds[kind]
}
