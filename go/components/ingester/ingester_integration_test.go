//go:build integration

package ingester_test

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uniqueName returns a test-scoped name that won't collide across runs.
// MySQL rows from a previous run have a different UID (PK), so a repeated
// name causes duplicate rows and breaks single-value assertions.
func uniqueName(base string) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return fmt.Sprintf("%s-%s", base, b)
}

// Run with: go test -mod=mod -tags integration ./components/ingester/ -v -run TestIngester
//
// Prerequisites:
//   - kubectl context pointing at michelangelo-sandbox k3d cluster
//   - michelangelo-controllermgr pod running with metadata storage enabled
//   - MySQL reachable via kubectl exec on the "mysql" pod in namespace "default"

const (
	testNamespace = "ma-dev-test"
	mysqlPod      = "mysql"
	mysqlNS       = "default"
	mysqlDB       = "michelangelo"
	mysqlUser     = "root"
	mysqlPass     = "root"

	finalizerName = "michelangelo/Ingester"
	pollInterval  = 500 * time.Millisecond
	timeout       = 20 * time.Second
)

// kubectl runs kubectl with the given args and returns trimmed stdout.
func kubectl(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("kubectl", args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("kubectl %v: %v\nstderr: %s", args, err, errBuf.String())
	}
	return strings.TrimSpace(out.String())
}

// kubectlMayFail runs kubectl and returns (stdout, error) without failing the test.
func kubectlMayFail(args ...string) (string, error) {
	cmd := exec.Command("kubectl", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// kubectlApply applies a manifest from stdin. Uses --validate=false so client-side
// schema download is skipped — the API server still validates the object.
func kubectlApply(t *testing.T, manifest string) {
	t.Helper()
	cmd := exec.Command("kubectl", "apply", "--validate=false", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "kubectl apply failed: %s", out)
}

// mysqlQuery runs a SQL query via kubectl exec on the MySQL pod.
func mysqlQuery(t *testing.T, query string) string {
	t.Helper()
	cmd := exec.Command("kubectl", "exec", "-n", mysqlNS, mysqlPod, "--",
		"mysql", "-u", mysqlUser, fmt.Sprintf("-p%s", mysqlPass), mysqlDB,
		"-s", "-e", query)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("mysql query %q: %v\nstderr: %s", query, err, errBuf.String())
	}
	// Strip the "mysql: [Warning]..." line that appears on stderr (already discarded).
	return strings.TrimSpace(out.String())
}

// waitFor polls cond every pollInterval until it returns true or timeout expires.
func waitFor(t *testing.T, desc string, cond func() bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		if cond() {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for: %s", desc)
		case <-time.After(pollInterval):
		}
	}
}

// existsInK8s returns true if the resource is found in ETCD via kubectl get.
func existsInK8s(kind, name, namespace string) bool {
	_, err := kubectlMayFail("get", kind, name, "-n", namespace)
	return err == nil
}

// rowInMySQL returns the value of a single column for a row identified by name+namespace.
func rowInMySQL(t *testing.T, table, column, name, namespace string) string {
	t.Helper()
	q := fmt.Sprintf("SELECT %s FROM %s WHERE name='%s' AND namespace='%s';",
		column, table, name, namespace)
	return mysqlQuery(t, q)
}

// cleanup deletes the resource if it still exists; ignores not-found errors.
func cleanup(kind, name, namespace string) {
	kubectlMayFail("delete", kind, name, "-n", namespace, "--ignore-not-found")
}

// TestIngester_ImmutableKind_MovedToMySQLOnly creates a Model (immutable kind)
// and verifies it lands in MySQL and is removed from ETCD.
func TestIngester_ImmutableKind_MovedToMySQLOnly(t *testing.T) {
	name := uniqueName("it-immutable-kind")
	t.Cleanup(func() { cleanup("model", name, testNamespace) })

	manifest := fmt.Sprintf(`
apiVersion: michelangelo.api/v2
kind: Model
metadata:
  name: %s
  namespace: %s
spec:
  description: "integration test - immutable kind"
`, name, testNamespace)

	kubectlApply(t, manifest)

	// Must be gone from ETCD.
	waitFor(t, "model removed from ETCD", func() bool {
		return !existsInK8s("model", name, testNamespace)
	})

	// Must be present in MySQL with no delete_time.
	waitFor(t, "model row in MySQL", func() bool {
		return rowInMySQL(t, "model", "name", name, testNamespace) == name
	})
	assert.Equal(t, "NULL", rowInMySQL(t, "model", "delete_time", name, testNamespace),
		"immutable model should not be soft-deleted in MySQL")
}

// TestIngester_MutableKind_FinalizerAdded creates a Pipeline (mutable) and
// verifies the ingester finalizer is set and the row is upserted to MySQL.
func TestIngester_MutableKind_FinalizerAdded(t *testing.T) {
	name := uniqueName("it-mutable-finalizer")
	t.Cleanup(func() { cleanup("pipeline", name, testNamespace) })

	manifest := fmt.Sprintf(`
apiVersion: michelangelo.api/v2
kind: Pipeline
metadata:
  name: %s
  namespace: %s
spec:
  description: "integration test - mutable with finalizer"
`, name, testNamespace)

	kubectlApply(t, manifest)

	// Finalizer must be added while the object stays in ETCD.
	waitFor(t, "ingester finalizer present", func() bool {
		finalizers, _ := kubectlMayFail("get", "pipeline", name, "-n", testNamespace,
			"-o", "jsonpath={.metadata.finalizers}")
		return strings.Contains(finalizers, finalizerName)
	})

	// Object must still be in ETCD (mutable kinds stay there).
	assert.True(t, existsInK8s("pipeline", name, testNamespace),
		"mutable pipeline should remain in ETCD")

	// Row must exist in MySQL.
	waitFor(t, "pipeline row in MySQL", func() bool {
		return rowInMySQL(t, "pipeline", "name", name, testNamespace) == name
	})
	assert.Equal(t, "NULL", rowInMySQL(t, "pipeline", "delete_time", name, testNamespace))
}

// TestIngester_MutableKind_DeletionTimestamp_SetsDeleteTime creates a Pipeline,
// deletes it via kubectl, and verifies MySQL delete_time is set and the object
// leaves ETCD cleanly (finalizer removed, no UID precondition errors).
func TestIngester_MutableKind_DeletionTimestamp_SetsDeleteTime(t *testing.T) {
	name := uniqueName("it-deletion-ts")
	t.Cleanup(func() { cleanup("pipeline", name, testNamespace) })

	manifest := fmt.Sprintf(`
apiVersion: michelangelo.api/v2
kind: Pipeline
metadata:
  name: %s
  namespace: %s
spec:
  description: "integration test - deletion via DeletionTimestamp"
`, name, testNamespace)

	kubectlApply(t, manifest)

	// Wait for finalizer before deleting.
	waitFor(t, "ingester finalizer present before delete", func() bool {
		finalizers, _ := kubectlMayFail("get", "pipeline", name, "-n", testNamespace,
			"-o", "jsonpath={.metadata.finalizers}")
		return strings.Contains(finalizers, finalizerName)
	})

	// kubectl delete triggers DeletionTimestamp.
	kubectl(t, "delete", "pipeline", name, "-n", testNamespace)

	// Object must leave ETCD.
	waitFor(t, "pipeline removed from ETCD", func() bool {
		return !existsInK8s("pipeline", name, testNamespace)
	})

	// MySQL row must have delete_time set (not NULL).
	waitFor(t, "pipeline delete_time set in MySQL", func() bool {
		v := rowInMySQL(t, "pipeline", "delete_time IS NOT NULL", name, testNamespace)
		return v == "1"
	})
}

// TestIngester_DeletingAnnotation_DeletesFromBoth creates a Pipeline, annotates it
// with michelangelo/Deleting=true, and verifies it disappears from both ETCD and
// MySQL (delete_time set).
func TestIngester_DeletingAnnotation_DeletesFromBoth(t *testing.T) {
	name := uniqueName("it-deleting-ann")
	t.Cleanup(func() { cleanup("pipeline", name, testNamespace) })

	manifest := fmt.Sprintf(`
apiVersion: michelangelo.api/v2
kind: Pipeline
metadata:
  name: %s
  namespace: %s
spec:
  description: "integration test - annotation deletion"
`, name, testNamespace)

	kubectlApply(t, manifest)

	// Wait for finalizer.
	waitFor(t, "ingester finalizer present before annotation", func() bool {
		finalizers, _ := kubectlMayFail("get", "pipeline", name, "-n", testNamespace,
			"-o", "jsonpath={.metadata.finalizers}")
		return strings.Contains(finalizers, finalizerName)
	})

	// Annotate to trigger annotation-based deletion.
	kubectl(t, "annotate", "pipeline", name, "-n", testNamespace,
		"michelangelo/Deleting=true")

	// Must leave ETCD.
	waitFor(t, "pipeline removed from ETCD after annotation", func() bool {
		return !existsInK8s("pipeline", name, testNamespace)
	})

	// MySQL delete_time must be set.
	waitFor(t, "pipeline delete_time set after annotation deletion", func() bool {
		v := rowInMySQL(t, "pipeline", "delete_time IS NOT NULL", name, testNamespace)
		return v == "1"
	})
}

// TestIngester_ImmutableAnnotation_MovedToMySQLOnly annotates a mutable Deployment
// with michelangelo/Immutable=true and verifies it is moved to MySQL and removed from ETCD.
func TestIngester_ImmutableAnnotation_MovedToMySQLOnly(t *testing.T) {
	name := uniqueName("it-immutable-ann")
	t.Cleanup(func() { cleanup("deployment", name, testNamespace) })

	manifest := fmt.Sprintf(`
apiVersion: michelangelo.api/v2
kind: Deployment
metadata:
  name: %s
  namespace: %s
  annotations:
    michelangelo/Immutable: "true"
`, name, testNamespace)

	kubectlApply(t, manifest)

	// Must be gone from ETCD.
	waitFor(t, "deployment removed from ETCD via immutable annotation", func() bool {
		return !existsInK8s("deployment", name, testNamespace)
	})

	// Must be in MySQL without delete_time.
	waitFor(t, "deployment row in MySQL", func() bool {
		return rowInMySQL(t, "deployment", "name", name, testNamespace) == name
	})
	assert.Equal(t, "NULL", rowInMySQL(t, "deployment", "delete_time", name, testNamespace))
}
