package mysql

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func newSchemeWithV2(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(s))
	return s
}

func TestGetTableName_GVKPopulated(t *testing.T) {
	m := &mysqlMetadataStorage{scheme: newSchemeWithV2(t)}
	obj := &v2pb.TriggerRun{
		TypeMeta: metav1.TypeMeta{Kind: "TriggerRun", APIVersion: "michelangelo.api/v2"},
	}
	require.Equal(t, "triggerrun", m.getTableName(obj))
}

func TestGetTableName_GVKEmpty_SchemeFallback(t *testing.T) {
	// controller-runtime returns objects with empty TypeMeta (issue #1517);
	// the scheme fallback must resolve the Kind from the registered type.
	m := &mysqlMetadataStorage{scheme: newSchemeWithV2(t)}
	obj := &v2pb.TriggerRun{}
	require.Equal(t, "triggerrun", m.getTableName(obj))
}

func TestGetTableName_GVKEmpty_NilScheme(t *testing.T) {
	// No scheme configured + empty GVK = "" (caller decides). Must not panic.
	m := &mysqlMetadataStorage{scheme: nil}
	obj := &v2pb.TriggerRun{}
	require.Equal(t, "", m.getTableName(obj))
}

func TestGetTableName_GVKEmpty_UnknownToScheme(t *testing.T) {
	// Scheme exists but doesn't know this type → falls through to "".
	m := &mysqlMetadataStorage{scheme: runtime.NewScheme()}
	obj := &v2pb.TriggerRun{}
	require.Equal(t, "", m.getTableName(obj))
}
