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
	"sort"
	"testing"
	"time"

	api "github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// TestIntegration_DeleteThenRecreate_NoOrphanRow reproduces (pre-fix) and
// verifies the fix (post-fix) for the orphaned-row scenario raised in review on
// #1332: a CR deleted outside michelangelo's own API (kubectl/GitOps, no
// DeletingAnnotation) and recreated with the same name/namespace gets a new
// K8s UID. Because `uid` is the MySQL primary key, the recreated object
// inserts a distinct row instead of colliding with the old one. The ingester
// fix (handleCascadeDeletion) now soft-deletes the old row via storage.Delete
// before the finalizer is removed, which this test simulates directly against
// the storage layer: Upsert(cr1) -> Delete(cr1) -> Upsert(cr2, new UID) should
// leave exactly one live row, with the old row present but soft-deleted
// (not a live duplicate).
func TestIntegration_DeleteThenRecreate_NoOrphanRow(t *testing.T) {
	db := openIntegrationDB(t)
	store := newIntegrationStorage(t, db)

	const (
		ns   = "default"
		name = "orphan-repro-deployment"
	)
	uid1 := string(types.UID("orig-uid-" + name))
	uid2 := string(types.UID("new-uid-" + name))
	t.Cleanup(func() { cleanupDeploymentRow(t, db, ns, name, uid1, uid2) })

	typeMeta := metav1.TypeMeta{Kind: "Deployment", APIVersion: "michelangelo.uber.com/v2"}

	cr1 := &v2pb.Deployment{
		TypeMeta:   typeMeta,
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: types.UID(uid1), ResourceVersion: "1", CreationTimestamp: metav1.Now()},
	}
	require.NoError(t, store.Upsert(context.Background(), cr1, false, nil))

	// Simulate what handleCascadeDeletion now does on a plain kubectl delete of
	// a non-opted-in kind: soft-delete the row before the finalizer is removed.
	require.NoError(t, store.Delete(context.Background(), &typeMeta, ns, name))

	// Recreate: k8s assigns a new UID.
	cr2 := &v2pb.Deployment{
		TypeMeta:   typeMeta,
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: types.UID(uid2), ResourceVersion: "2", CreationTimestamp: metav1.Now()},
	}
	require.NoError(t, store.Upsert(context.Background(), cr2, false, nil))

	// Exactly one live row should remain for this namespace/name.
	var liveCount int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM deployment WHERE namespace = ? AND name = ? AND delete_time IS NULL",
		ns, name,
	).Scan(&liveCount))
	assert.Equal(t, 1, liveCount, "exactly one live row should remain after delete+recreate")

	// The old row still physically exists (soft delete, not a hard delete) but
	// is marked deleted, not a live duplicate.
	var oldDeleteTime sql.NullTime
	require.NoError(t, db.QueryRow("SELECT delete_time FROM deployment WHERE uid = ?", uid1).Scan(&oldDeleteTime))
	assert.True(t, oldDeleteTime.Valid, "old row should be soft-deleted (delete_time set)")

	// The new row is live.
	var newDeleteTime sql.NullTime
	require.NoError(t, db.QueryRow("SELECT delete_time FROM deployment WHERE uid = ?", uid2).Scan(&newDeleteTime))
	assert.False(t, newDeleteTime.Valid, "new row should be live (delete_time NULL)")
}

// TestIntegration_DeleteThenRecreate_WithoutFix_WouldOrphan documents the bug
// this fix addresses: skipping the Delete call (the pre-fix behavior for
// non-opted-in kinds) leaves the old row live under its own UID forever, so
// namespace/name now maps to two live rows.
func TestIntegration_DeleteThenRecreate_WithoutFix_WouldOrphan(t *testing.T) {
	db := openIntegrationDB(t)
	store := newIntegrationStorage(t, db)

	const (
		ns   = "default"
		name = "orphan-repro-no-fix-deployment"
	)
	uid1 := string(types.UID("orig-uid-" + name))
	uid2 := string(types.UID("new-uid-" + name))
	t.Cleanup(func() { cleanupDeploymentRow(t, db, ns, name, uid1, uid2) })

	typeMeta := metav1.TypeMeta{Kind: "Deployment", APIVersion: "michelangelo.uber.com/v2"}

	cr1 := &v2pb.Deployment{
		TypeMeta:   typeMeta,
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: types.UID(uid1), ResourceVersion: "1", CreationTimestamp: metav1.Now()},
	}
	require.NoError(t, store.Upsert(context.Background(), cr1, false, nil))

	// No Delete call here — this is the pre-fix `handleCascadeDeletion` no-op
	// for non-opted-in kinds on a plain kubectl delete.

	cr2 := &v2pb.Deployment{
		TypeMeta:   typeMeta,
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: types.UID(uid2), ResourceVersion: "2", CreationTimestamp: metav1.Now()},
	}
	require.NoError(t, store.Upsert(context.Background(), cr2, false, nil))

	var liveCount int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM deployment WHERE namespace = ? AND name = ? AND delete_time IS NULL",
		ns, name,
	).Scan(&liveCount))
	assert.Equal(t, 2, liveCount, "without the Delete call, both rows are live orphans for the same namespace/name")
}

// TestIntegration_List_OrderBy_InjectionPayloadRejected is the smoke test for the
// buildOrderBySQL fix (spec 018): runs the real List() RPC path — not the pure-function
// unit test — against the pre-existing "california-housing" pipeline_run rows already
// live in the local k3d sandbox's MySQL (produced by real sandbox pipeline runs, not
// synthesized here), with a malicious order_by field. Confirms the query is rejected
// with InvalidArgument before ever reaching the database, and that the row count is
// unaffected (i.e. the payload's DROP TABLE / SLEEP side effects never executed).
func TestIntegration_List_OrderBy_InjectionPayloadRejected(t *testing.T) {
	db := openIntegrationDB(t)
	store := newIntegrationStorage(t, db)
	ctx := context.Background()

	var countBefore int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pipeline_run").Scan(&countBefore))
	require.Positive(t, countBefore, "expected pre-existing real pipeline_run rows from prior sandbox runs")

	typeMeta := &metav1.TypeMeta{Kind: "PipelineRun", APIVersion: "michelangelo.api/v2"}
	listOptsExt := &apipb.ListOptionsExt{
		OrderBy: []*apipb.OrderBy{
			{Field: "name` = (SELECT SLEEP(5))-- -", Dir: apipb.SORT_ORDER_ASC},
		},
	}
	var listResp storage.ListResponse
	err := store.List(ctx, typeMeta, "california-housing", &metav1.ListOptions{}, listOptsExt, &listResp)
	require.Error(t, err, "malicious order_by field must be rejected, not reach the database")
	assert.Contains(t, err.Error(), "invalid order_by field")

	var countAfter int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pipeline_run").Scan(&countAfter))
	assert.Equal(t, countBefore, countAfter, "rejected payload must have no side effect on the table")
}

// TestIntegration_List_OrderBy_ValidFieldStillWorks confirms the fix doesn't regress
// legitimate ORDER BY usage: sorts the same real pipeline_run rows by create_time in
// both directions and asserts the real, non-synthetic rows come back in the expected
// chronological order.
func TestIntegration_List_OrderBy_ValidFieldStillWorks(t *testing.T) {
	db := openIntegrationDB(t)
	store := newIntegrationStorage(t, db)
	ctx := context.Background()

	typeMeta := &metav1.TypeMeta{Kind: "PipelineRun", APIVersion: "michelangelo.api/v2"}

	ascOpts := &apipb.ListOptionsExt{
		OrderBy: []*apipb.OrderBy{{Field: "create_time", Dir: apipb.SORT_ORDER_ASC}},
	}
	var ascResp storage.ListResponse
	require.NoError(t, store.List(ctx, typeMeta, "california-housing", &metav1.ListOptions{}, ascOpts, &ascResp))
	require.GreaterOrEqual(t, len(ascResp.Items), 5, "expected the real pre-existing california-housing pipeline runs")

	ascNames := make([]string, len(ascResp.Items))
	for i, item := range ascResp.Items {
		pr, ok := item.(*v2pb.PipelineRun)
		require.True(t, ok)
		ascNames[i] = pr.GetName()
	}
	assert.True(t, sort.StringsAreSorted(ascNames), "ASC order_by=create_time must return rows in ascending chronological order: %v", ascNames)

	descOpts := &apipb.ListOptionsExt{
		OrderBy: []*apipb.OrderBy{{Field: "create_time", Dir: apipb.SORT_ORDER_DESC}},
	}
	var descResp storage.ListResponse
	require.NoError(t, store.List(ctx, typeMeta, "california-housing", &metav1.ListOptions{}, descOpts, &descResp))
	require.Equal(t, len(ascResp.Items), len(descResp.Items))

	descNames := make([]string, len(descResp.Items))
	for i, item := range descResp.Items {
		pr, ok := item.(*v2pb.PipelineRun)
		require.True(t, ok)
		descNames[i] = pr.GetName()
	}
	for i := range ascNames {
		assert.Equal(t, ascNames[i], descNames[len(descNames)-1-i], "DESC must be the exact reverse of ASC over the same real rows")
	}
}

// ==============================================================================
// directUpdate — the generic Update RPC path (issue #1622)
// ==============================================================================

// newDirectUpdateDeployment builds a Deployment CR for the directUpdate tests.
func newDirectUpdateDeployment(namespace, name, uid, resVersion string,
	labels, annotations map[string]string) *v2pb.Deployment {
	return &v2pb.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "michelangelo.uber.com/v2",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			UID:               types.UID(uid),
			ResourceVersion:   resVersion,
			CreationTimestamp: metav1.Now(),
			Labels:            labels,
			Annotations:       annotations,
		},
	}
}

// seedDeployment performs the non-direct Upsert a real ingester sync would do, so the
// direct-update tests have a live row to work against.
func seedDeployment(t *testing.T, store *mysqlMetadataStorage, namespace, name, uid, resVersion string,
	labels, annotations map[string]string) *v2pb.Deployment {
	t.Helper()
	cr := newDirectUpdateDeployment(namespace, name, uid, resVersion, labels, annotations)
	require.NoError(t, store.Upsert(context.Background(), cr, false, nil), "seed Upsert must succeed")
	return cr
}

// storedResVersion reads the res_version column straight out of the row.
func storedResVersion(t *testing.T, db *sql.DB, namespace, name string) uint64 {
	t.Helper()
	var rv uint64
	require.NoError(t,
		db.QueryRow("SELECT res_version FROM deployment WHERE namespace=? AND name=? AND delete_time IS NULL",
			namespace, name).Scan(&rv))
	return rv
}

// TestIntegration_DirectUpdate_HappyPath is the base case: a direct update must succeed,
// bump the stored resource version, and — critically — actually commit. Before the fix
// the direct branch of Upsert returned without ever calling tx.Commit(), so the column
// assertion here is what proves the transaction landed.
func TestIntegration_DirectUpdate_HappyPath(t *testing.T) {
	db := openIntegrationDB(t)
	store := newIntegrationStorage(t, db)
	ctx := context.Background()

	const (
		namespace = "integration-test-ns"
		name      = "direct-update-happy"
		uid       = "direct-update-happy-uid"
	)
	t.Cleanup(func() { cleanupDeploymentRow(t, db, namespace, name, uid) })

	seedDeployment(t, store, namespace, name, uid, "1", map[string]string{"app": "before"}, nil)

	var createTimeBefore, updateTimeBefore time.Time
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT create_time, update_time FROM deployment WHERE uid=?", uid).
		Scan(&createTimeBefore, &updateTimeBefore))

	update := newDirectUpdateDeployment(namespace, name, "", "1", map[string]string{"app": "after"}, nil)
	require.NoError(t, store.Upsert(ctx, update, true, nil), "direct update must succeed")

	assert.Equal(t, uint64(2), storedResVersion(t, db, namespace, name),
		"res_version must be bumped and the transaction committed")
	assert.Equal(t, "2", update.GetResourceVersion(),
		"the new resource version must be written back onto the caller's object")

	var createTimeAfter, updateTimeAfter time.Time
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT create_time, update_time FROM deployment WHERE uid=?", uid).
		Scan(&createTimeAfter, &updateTimeAfter))
	assert.Equal(t, createTimeBefore.UTC(), createTimeAfter.UTC(), "create_time must not move")
	assert.False(t, updateTimeAfter.Before(updateTimeBefore), "update_time must not go backwards")
}

// TestIntegration_DirectUpdate_RewritesProtoSoNextReadSeesNewRV guards the subtle half of
// #1622: GetByName reconstructs the object from the proto blob alone, so bumping only the
// res_version column would leave the next read reporting a stale version.
func TestIntegration_DirectUpdate_RewritesProtoSoNextReadSeesNewRV(t *testing.T) {
	db := openIntegrationDB(t)
	store := newIntegrationStorage(t, db)
	ctx := context.Background()

	const (
		namespace = "integration-test-ns"
		name      = "direct-update-proto-rewrite"
		uid       = "direct-update-proto-rewrite-uid"
	)
	t.Cleanup(func() { cleanupDeploymentRow(t, db, namespace, name, uid) })

	seedDeployment(t, store, namespace, name, uid, "1", map[string]string{"app": "before"}, nil)

	update := newDirectUpdateDeployment(namespace, name, "", "1", map[string]string{"app": "after"}, nil)
	require.NoError(t, store.Upsert(ctx, update, true, nil))

	readBack := &v2pb.Deployment{}
	require.NoError(t, store.GetByName(ctx, namespace, name, readBack))
	assert.Equal(t, "2", readBack.GetResourceVersion(),
		"GetByName reads metadata from the proto blob, so the blob must carry the new version")
	assert.Equal(t, "after", readBack.GetLabels()["app"],
		"the merged labels must be persisted into the proto blob too")
}

// TestIntegration_DirectUpdate_RepeatedReadModifyWriteSucceeds is the #1622 repro reduced
// to the storage layer: the register_model client re-reads the object and reuses its
// resource version on every retry. Three successive cycles must all succeed. Without the
// proto rewrite the third one fails with FailedPrecondition forever.
func TestIntegration_DirectUpdate_RepeatedReadModifyWriteSucceeds(t *testing.T) {
	db := openIntegrationDB(t)
	store := newIntegrationStorage(t, db)
	ctx := context.Background()

	const (
		namespace = "integration-test-ns"
		name      = "direct-update-repeat"
		uid       = "direct-update-repeat-uid"
	)
	t.Cleanup(func() { cleanupDeploymentRow(t, db, namespace, name, uid) })

	seedDeployment(t, store, namespace, name, uid, "1", map[string]string{"attempt": "0"}, nil)

	for i := 1; i <= 3; i++ {
		existing := &v2pb.Deployment{}
		require.NoError(t, store.GetByName(ctx, namespace, name, existing),
			"read before update %d", i)

		update := newDirectUpdateDeployment(namespace, name, "", existing.GetResourceVersion(),
			map[string]string{"attempt": fmt.Sprintf("%d", i)}, nil)
		require.NoError(t, store.Upsert(ctx, update, true, nil),
			"read-modify-write cycle %d must succeed", i)
	}

	assert.Equal(t, uint64(4), storedResVersion(t, db, namespace, name))
	final := &v2pb.Deployment{}
	require.NoError(t, store.GetByName(ctx, namespace, name, final))
	assert.Equal(t, "3", final.GetLabels()["attempt"])
}

// TestIntegration_DirectUpdate_EmptyResourceVersionIsUnconditional mirrors Kubernetes
// semantics: an unset resourceVersion on update means "no precondition".
func TestIntegration_DirectUpdate_EmptyResourceVersionIsUnconditional(t *testing.T) {
	db := openIntegrationDB(t)
	store := newIntegrationStorage(t, db)
	ctx := context.Background()

	const (
		namespace = "integration-test-ns"
		name      = "direct-update-empty-rv"
		uid       = "direct-update-empty-rv-uid"
	)
	t.Cleanup(func() { cleanupDeploymentRow(t, db, namespace, name, uid) })

	seedDeployment(t, store, namespace, name, uid, "5", map[string]string{"app": "before"}, nil)

	update := newDirectUpdateDeployment(namespace, name, "", "", map[string]string{"app": "after"}, nil)
	require.NoError(t, store.Upsert(ctx, update, true, nil), "empty resourceVersion must not be a precondition")
	assert.Equal(t, uint64(6), storedResVersion(t, db, namespace, name))
}

// TestIntegration_DirectUpdate_ResourceVersionMismatchConflicts checks both halves of the
// optimistic-concurrency contract: the right gRPC code out, and no partial write left
// behind (which also proves the deferred rollback still works).
func TestIntegration_DirectUpdate_ResourceVersionMismatchConflicts(t *testing.T) {
	db := openIntegrationDB(t)
	store := newIntegrationStorage(t, db)
	ctx := context.Background()

	const (
		namespace = "integration-test-ns"
		name      = "direct-update-conflict"
		uid       = "direct-update-conflict-uid"
	)
	t.Cleanup(func() { cleanupDeploymentRow(t, db, namespace, name, uid) })

	seedDeployment(t, store, namespace, name, uid, "5", map[string]string{"app": "before"}, nil)

	update := newDirectUpdateDeployment(namespace, name, "", "4", map[string]string{"app": "after"}, nil)
	err := store.Upsert(ctx, update, true, nil)
	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err),
		"a stale resourceVersion must surface as FailedPrecondition: the Python client "+
			"catches exactly this code to drive its retry loop")

	assert.Equal(t, uint64(5), storedResVersion(t, db, namespace, name), "row must be unchanged")
	unchanged := &v2pb.Deployment{}
	require.NoError(t, store.GetByName(ctx, namespace, name, unchanged))
	assert.Equal(t, "before", unchanged.GetLabels()["app"], "labels must be unchanged")
}

// TestIntegration_DirectUpdate_NonNumericResourceVersion rejects a structurally invalid
// version as InvalidArgument rather than treating it as a lost update.
func TestIntegration_DirectUpdate_NonNumericResourceVersion(t *testing.T) {
	db := openIntegrationDB(t)
	store := newIntegrationStorage(t, db)
	ctx := context.Background()

	const (
		namespace = "integration-test-ns"
		name      = "direct-update-bad-rv"
		uid       = "direct-update-bad-rv-uid"
	)
	t.Cleanup(func() { cleanupDeploymentRow(t, db, namespace, name, uid) })

	seedDeployment(t, store, namespace, name, uid, "5", nil, nil)

	update := newDirectUpdateDeployment(namespace, name, "", "not-a-number", nil, nil)
	err := store.Upsert(ctx, update, true, nil)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Equal(t, uint64(5), storedResVersion(t, db, namespace, name), "row must be unchanged")
}

// TestIntegration_DirectUpdate_RowNotFound covers a namespace/name that was never stored.
func TestIntegration_DirectUpdate_RowNotFound(t *testing.T) {
	db := openIntegrationDB(t)
	store := newIntegrationStorage(t, db)
	ctx := context.Background()

	update := newDirectUpdateDeployment("integration-test-ns", "direct-update-never-existed", "", "1", nil, nil)
	err := store.Upsert(ctx, update, true, nil)
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// TestIntegration_DirectUpdate_SoftDeletedRowNotFound verifies a soft-deleted row cannot
// be resurrected through the direct path.
func TestIntegration_DirectUpdate_SoftDeletedRowNotFound(t *testing.T) {
	db := openIntegrationDB(t)
	store := newIntegrationStorage(t, db)
	ctx := context.Background()

	const (
		namespace = "integration-test-ns"
		name      = "direct-update-soft-deleted"
		uid       = "direct-update-soft-deleted-uid"
	)
	t.Cleanup(func() { cleanupDeploymentRow(t, db, namespace, name, uid) })

	seedDeployment(t, store, namespace, name, uid, "1", nil, nil)
	typeMeta := &metav1.TypeMeta{APIVersion: "michelangelo.uber.com/v2", Kind: "Deployment"}
	require.NoError(t, store.Delete(ctx, typeMeta, namespace, name))

	update := newDirectUpdateDeployment(namespace, name, "", "1", nil, nil)
	err := store.Upsert(ctx, update, true, nil)
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))

	var deleteTime sql.NullTime
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT delete_time FROM deployment WHERE uid=?", uid).Scan(&deleteTime))
	assert.True(t, deleteTime.Valid, "the row must stay soft-deleted")
}

// TestIntegration_DirectUpdate_ChildRowsKeyedByStoredUID is the highest-value case: the
// register_model client sends name/namespace/labels but never a UID, so keying child rows
// off metadataStoragePrimaryKey(incoming) would file them all under obj_uid = ”.
func TestIntegration_DirectUpdate_ChildRowsKeyedByStoredUID(t *testing.T) {
	db := openIntegrationDB(t)
	store := newIntegrationStorage(t, db)
	ctx := context.Background()

	const (
		namespace = "integration-test-ns"
		name      = "direct-update-child-rows"
		uid       = "direct-update-child-rows-uid"
	)
	t.Cleanup(func() { cleanupDeploymentRow(t, db, namespace, name, uid, "") })

	seedDeployment(t, store, namespace, name, uid, "1",
		map[string]string{"app": "before", "team": "platform"}, nil)

	// No UID and no primary-key annotation — exactly what the Python client sends.
	update := newDirectUpdateDeployment(namespace, name, "", "1", map[string]string{"app": "after"}, nil)
	require.NoError(t, store.Upsert(ctx, update, true, nil))

	rows, err := db.QueryContext(ctx,
		"SELECT `key`, `value` FROM deployment_labels WHERE obj_uid=? ORDER BY `key`", uid)
	require.NoError(t, err)
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var k, v string
		require.NoError(t, rows.Scan(&k, &v))
		got[k] = v
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, map[string]string{"app": "after"}, got,
		"labels must be replaced wholesale and keyed by the stored uid")

	var orphaned int
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM deployment_labels WHERE obj_uid=''").Scan(&orphaned))
	assert.Equal(t, 0, orphaned, "no label rows may be orphaned under an empty obj_uid")
}

// TestIntegration_DirectUpdate_PreservesManagedAnnotations verifies that server-managed
// annotations survive a caller that builds a fresh object and never echoes them back.
func TestIntegration_DirectUpdate_PreservesManagedAnnotations(t *testing.T) {
	db := openIntegrationDB(t)
	store := newIntegrationStorage(t, db)
	ctx := context.Background()

	const (
		namespace = "integration-test-ns"
		name      = "direct-update-managed-annotations"
		uid       = "direct-update-managed-annotations-uid"
	)
	t.Cleanup(func() { cleanupDeploymentRow(t, db, namespace, name, uid) })

	seedDeployment(t, store, namespace, name, uid, "1", nil, map[string]string{
		api.ImmutableAnnotation:                 "true",
		api.MetadataStoragePrimaryKeyAnnotation: uid,
	})

	update := newDirectUpdateDeployment(namespace, name, "", "1", nil,
		map[string]string{"user-annotation": "set-by-client"})
	require.NoError(t, store.Upsert(ctx, update, true, nil))

	readBack := &v2pb.Deployment{}
	require.NoError(t, store.GetByName(ctx, namespace, name, readBack))
	annotations := readBack.GetAnnotations()
	assert.Equal(t, "true", annotations[api.ImmutableAnnotation],
		"the immutability marker must not be destroyed by a metadata-only update")
	assert.Equal(t, uid, annotations[api.MetadataStoragePrimaryKeyAnnotation],
		"the migration primary key must not be destroyed by a metadata-only update")
	assert.Equal(t, "set-by-client", annotations["user-annotation"])
}

// TestIntegration_DirectUpdate_IgnoresSpecChanges locks in the deliberate contract
// behaviour: the direct path is metadata-only, so a caller that submits a modified spec
// has that part of its request silently dropped.
func TestIntegration_DirectUpdate_IgnoresSpecChanges(t *testing.T) {
	db := openIntegrationDB(t)
	store := newIntegrationStorage(t, db)
	ctx := context.Background()

	const (
		namespace = "integration-test-ns"
		name      = "direct-update-spec-ignored"
		uid       = "direct-update-spec-ignored-uid"
	)
	t.Cleanup(func() { cleanupDeploymentRow(t, db, namespace, name, uid) })

	seeded := newDirectUpdateDeployment(namespace, name, uid, "1", nil, nil)
	seeded.Spec.DesiredRevision = &apipb.ResourceIdentifier{Namespace: namespace, Name: "revision-stored"}
	require.NoError(t, store.Upsert(ctx, seeded, false, nil))

	update := newDirectUpdateDeployment(namespace, name, "", "1", map[string]string{"app": "after"}, nil)
	update.Spec.DesiredRevision = &apipb.ResourceIdentifier{Namespace: namespace, Name: "revision-from-caller"}
	require.NoError(t, store.Upsert(ctx, update, true, nil))

	readBack := &v2pb.Deployment{}
	require.NoError(t, store.GetByName(ctx, namespace, name, readBack))
	require.NotNil(t, readBack.Spec.DesiredRevision)
	assert.Equal(t, "revision-stored", readBack.Spec.DesiredRevision.Name,
		"the direct path must not persist spec changes")
	assert.Equal(t, "after", readBack.GetLabels()["app"], "but metadata changes must land")
}

// TestIntegration_DirectUpdate_EmptyStoredProtoRejected covers a corrupt row: directUpdate
// is a read-modify-write of the proto blob, and unmarshalling zero bytes succeeds with a
// zero-valued object. Writing that back would blank out the stored name, namespace and
// spec while the row's columns still looked correct, so an empty blob must abort.
func TestIntegration_DirectUpdate_EmptyStoredProtoRejected(t *testing.T) {
	db := openIntegrationDB(t)
	store := newIntegrationStorage(t, db)
	ctx := context.Background()

	const (
		namespace = "integration-test-ns"
		name      = "direct-update-empty-proto"
		uid       = "direct-update-empty-proto-uid"
	)
	t.Cleanup(func() { cleanupDeploymentRow(t, db, namespace, name, uid) })

	seedDeployment(t, store, namespace, name, uid, "1", nil, nil)
	_, err := db.ExecContext(ctx, "UPDATE deployment SET proto = NULL WHERE uid = ?", uid)
	require.NoError(t, err)

	update := newDirectUpdateDeployment(namespace, name, "", "1", map[string]string{"app": "after"}, nil)
	err = store.Upsert(ctx, update, true, nil)
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.Equal(t, uint64(1), storedResVersion(t, db, namespace, name), "row must be unchanged")
}
