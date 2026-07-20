package mysql

// Integration tests that require a live MySQL instance. They are skipped automatically
// when MySQL is unreachable, so they are safe to run in any environment.
//
// To run locally with the k3d sandbox:
//   MYSQL_HOST=localhost MYSQL_PORT=3306 go test -mod=mod -run TestIntegration ./storage/mysql/...

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	api "github.com/michelangelo-ai/michelangelo/go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// mysqlIntegrationConfig returns the Config for the local sandbox MySQL, reading
// MYSQL_HOST / MYSQL_PORT / MYSQL_USER / MYSQL_PASSWORD / MYSQL_DATABASE from env with
// safe defaults.
func mysqlIntegrationConfig() Config {
	host := os.Getenv("MYSQL_HOST")
	if host == "" {
		host = "localhost"
	}
	port := 3306
	if v := os.Getenv("MYSQL_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &port)
	}
	user := os.Getenv("MYSQL_USER")
	if user == "" {
		user = "root"
	}
	password := os.Getenv("MYSQL_PASSWORD")
	if password == "" {
		password = "root"
	}
	database := os.Getenv("MYSQL_DATABASE")
	if database == "" {
		database = "michelangelo"
	}
	return Config{Host: host, Port: port, User: user, Password: password, Database: database}
}

// openIntegrationDB connects to MySQL and returns the raw *sql.DB. The test is skipped
// (not failed) when MySQL is unreachable so the suite stays green in CI without a DB.
func openIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	cfg := mysqlIntegrationConfig()
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=UTC",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("skipping integration test: cannot open MySQL: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("skipping integration test: MySQL unreachable at %s:%d: %v", cfg.Host, cfg.Port, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newIntegrationStorage creates a mysqlMetadataStorage wired to the live DB.
func newIntegrationStorage(t *testing.T, db *sql.DB) *mysqlMetadataStorage {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(s))
	return &mysqlMetadataStorage{db: db, scheme: s}
}

// cleanupDeploymentRow removes rows created by the test so re-runs are idempotent.
// It clears the main table plus the label/annotation child tables for the given uids
// (child rows are keyed by obj_uid, not namespace/name, so they must be cleaned by uid).
func cleanupDeploymentRow(t *testing.T, db *sql.DB, namespace, name string, uids ...string) {
	t.Helper()
	_, err := db.Exec("DELETE FROM deployment WHERE namespace=? AND name=?", namespace, name)
	require.NoError(t, err)
	for _, uid := range uids {
		_, err = db.Exec("DELETE FROM deployment_labels WHERE obj_uid=?", uid)
		require.NoError(t, err)
		_, err = db.Exec("DELETE FROM deployment_annotations WHERE obj_uid=?", uid)
		require.NoError(t, err)
	}
}

// TestIntegration_MetadataStoragePrimaryKey_MigrationScenario exercises the full
// cluster-migration flow end-to-end against a live MySQL:
//
//  1. Create a CR with MetadataStoragePrimaryKeyAnnotation set to its own UID ("cluster 1").
//  2. Upsert to MySQL - verify a single row keyed by the original UID.
//  3. Simulate migration: same name, new k8s UID ("cluster 2"), annotation still points
//     to the original UID.
//  4. Upsert again - the UPSERT must hit the existing row (no duplicate).
//  5. Assert: still exactly one row, and its PK is the original UID.
func TestIntegration_MetadataStoragePrimaryKey_MigrationScenario(t *testing.T) {
	db := openIntegrationDB(t)
	store := newIntegrationStorage(t, db)

	const (
		namespace   = "integration-test-ns"
		name        = "migration-test-deployment"
		originalUID = "original-cluster-uid-aaa111"
		newUID      = "new-cluster-uid-bbb222"
	)

	t.Cleanup(func() { cleanupDeploymentRow(t, db, namespace, name, originalUID, newUID) })

	ctx := context.Background()

	// --- Step 1: Upsert with original UID (first cluster) ---
	cr1 := &v2pb.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "michelangelo.uber.com/v2",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			UID:               types.UID(originalUID),
			ResourceVersion:   "1",
			CreationTimestamp: metav1.Now(),
			Labels: map[string]string{
				"app": "migration-test",
			},
			Annotations: map[string]string{
				api.MetadataStoragePrimaryKeyAnnotation: originalUID,
			},
		},
	}
	require.NoError(t, store.Upsert(ctx, cr1, false, nil), "first Upsert must succeed")

	// Verify exactly one row exists keyed by the original UID.
	var storedUID string
	var rowCount int
	require.NoError(t,
		db.QueryRowContext(ctx, "SELECT uid FROM deployment WHERE namespace=? AND name=?", namespace, name).
			Scan(&storedUID),
		"row must exist after first Upsert")
	require.NoError(t,
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM deployment WHERE namespace=? AND name=?", namespace, name).
			Scan(&rowCount))
	assert.Equal(t, 1, rowCount, "exactly one row after first Upsert")
	assert.Equal(t, originalUID, storedUID, "PK must equal the original UID")

	// --- Step 2: Simulate migration - same name, new k8s UID, annotation preserved ---
	cr2 := &v2pb.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "michelangelo.uber.com/v2",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			UID:               types.UID(newUID), // new UID assigned by new cluster
			ResourceVersion:   "1",
			CreationTimestamp: metav1.Now(),
			Labels: map[string]string{
				"app": "migration-test",
			},
			Annotations: map[string]string{
				// operator preserved the annotation from the original CR
				api.MetadataStoragePrimaryKeyAnnotation: originalUID,
			},
		},
	}
	require.NoError(t, store.Upsert(ctx, cr2, false, nil), "second Upsert (migration) must succeed")

	// --- Step 3: Assert no duplicate row and stable PK ---
	require.NoError(t,
		db.QueryRowContext(ctx, "SELECT uid FROM deployment WHERE namespace=? AND name=?", namespace, name).
			Scan(&storedUID),
		"row must exist after migration Upsert")
	require.NoError(t,
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM deployment WHERE namespace=? AND name=?", namespace, name).
			Scan(&rowCount))
	assert.Equal(t, 1, rowCount, "still exactly one row after migration Upsert - no duplicate")
	assert.Equal(t, originalUID, storedUID, "PK must still equal the original UID, not the new cluster UID")

	// --- Step 4: Assert child rows stay keyed by the stable PK (no orphans) ---
	// The label/annotation child tables join back to the main row on obj_uid = uid.
	// After migration they must be keyed by the original UID (the stable PK), and there
	// must be no rows left under the new cluster UID.
	var childUnderOriginal, childUnderNew int
	require.NoError(t,
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM deployment_labels WHERE obj_uid=?", originalUID).
			Scan(&childUnderOriginal))
	assert.Equal(t, 1, childUnderOriginal, "label rows must be keyed by the stable PK (original UID)")
	require.NoError(t,
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM deployment_labels WHERE obj_uid=?", newUID).
			Scan(&childUnderNew))
	assert.Equal(t, 0, childUnderNew, "no label rows may be orphaned under the new cluster UID")

	require.NoError(t,
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM deployment_annotations WHERE obj_uid=?", originalUID).
			Scan(&childUnderOriginal))
	assert.Positive(t, childUnderOriginal, "annotation rows must be keyed by the stable PK (original UID)")
	require.NoError(t,
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM deployment_annotations WHERE obj_uid=?", newUID).
			Scan(&childUnderNew))
	assert.Equal(t, 0, childUnderNew, "no annotation rows may be orphaned under the new cluster UID")
}

// TestIntegration_MetadataStoragePrimaryKey_FallbackToUID verifies the backwards-compatible
// path: when the annotation is absent the storage uses the object's UID as PK, and a
// subsequent Upsert with the same UID updates the row in-place.
func TestIntegration_MetadataStoragePrimaryKey_FallbackToUID(t *testing.T) {
	db := openIntegrationDB(t)
	store := newIntegrationStorage(t, db)

	const (
		namespace = "integration-test-ns"
		name      = "fallback-uid-test-deployment"
		uid       = "fallback-uid-ccc333"
	)

	t.Cleanup(func() { cleanupDeploymentRow(t, db, namespace, name, uid) })

	ctx := context.Background()

	cr := &v2pb.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "michelangelo.uber.com/v2",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			UID:               types.UID(uid),
			ResourceVersion:   "1",
			CreationTimestamp: metav1.Now(),
			// No MetadataStoragePrimaryKeyAnnotation - old behavior
		},
	}
	require.NoError(t, store.Upsert(ctx, cr, false, nil))

	var storedUID string
	var rowCount int
	require.NoError(t,
		db.QueryRowContext(ctx, "SELECT uid FROM deployment WHERE namespace=? AND name=?", namespace, name).
			Scan(&storedUID))
	require.NoError(t,
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM deployment WHERE namespace=? AND name=?", namespace, name).
			Scan(&rowCount))
	assert.Equal(t, 1, rowCount, "one row when annotation is absent")
	assert.Equal(t, uid, storedUID, "PK falls back to the object UID when annotation is absent")
}
