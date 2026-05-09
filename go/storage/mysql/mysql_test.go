package mysql

import (
	"testing"

	gogotypes "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
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
	require.Equal(t, "trigger_run", m.getTableName(obj))
}

func TestGetTableName_GVKEmpty_SchemeFallback(t *testing.T) {
	// controller-runtime returns objects with empty TypeMeta (issue #1517);
	// the scheme fallback must resolve the Kind from the registered type.
	m := &mysqlMetadataStorage{scheme: newSchemeWithV2(t)}
	obj := &v2pb.TriggerRun{}
	require.Equal(t, "trigger_run", m.getTableName(obj))
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

// stringMatchValue wraps a string into a gogotypes.Any (StringValue).
func stringMatchValue(t *testing.T, s string) *gogotypes.Any {
	t.Helper()
	any, err := gogotypes.MarshalAny(&gogotypes.StringValue{Value: s})
	require.NoError(t, err)
	return any
}

func TestIsLabelField(t *testing.T) {
	require.True(t, isLabelField("pipeline_run.label.michelangelo/Foo"))
	require.False(t, isLabelField("pipeline_run.metadata.labels.michelangelo/Foo"))
	require.False(t, isLabelField("pipeline_run.state"))
	require.False(t, isLabelField("label"))
}

func TestIsLabelFieldInMetadata(t *testing.T) {
	require.True(t, isLabelFieldInMetadata("pipeline_run.metadata.labels.michelangelo/Foo"))
	require.False(t, isLabelFieldInMetadata("pipeline_run.label.michelangelo/Foo"))
	require.False(t, isLabelFieldInMetadata("pipeline_run.metadata.name"))
}

func TestProcessFieldName(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"pipeline_run.state", "state", false},
		{"pipeline_run.spec.foo", "spec.foo", false},
		{"pipeline_run.label.michelangelo/Foo", "michelangelo/Foo", false},
		{"pipeline_run.metadata.labels.michelangelo/Foo", "michelangelo/Foo", false},
		{"name", "", true}, // missing CRD prefix
	}
	for _, c := range cases {
		got, err := processFieldName(c.in)
		if c.wantErr {
			require.Error(t, err, "input %q", c.in)
			continue
		}
		require.NoError(t, err, "input %q", c.in)
		require.Equal(t, c.want, got, "input %q", c.in)
	}
}

func TestConvertCriterionOperator(t *testing.T) {
	// SQL fragments are byte-equivalent to the internal storage/pkg/mysql/lineage_util.go
	// convertCriterionOperator: leading space, trailing space on IS NULL/IS NOT NULL,
	// no spaces between placeholders in IN/NOT IN.
	cases := []struct {
		name       string
		op         apipb.CriterionOperator
		value      string
		wantSQL    string
		wantParams []interface{}
		wantErr    bool
	}{
		{"equal", apipb.CRITERION_OPERATOR_EQUAL, "alice", " `name` = ?", []interface{}{"alice"}, false},
		{"not_equal", apipb.CRITERION_OPERATOR_NOT_EQUAL, "alice", " `name` != ?", []interface{}{"alice"}, false},
		{"greater_than", apipb.CRITERION_OPERATOR_GREATER_THAN, "5", " `name` > ?", []interface{}{"5"}, false},
		{"gte", apipb.CRITERION_OPERATOR_GREATER_THAN_OR_EQUAL_TO, "5", " `name` >= ?", []interface{}{"5"}, false},
		{"less_than", apipb.CRITERION_OPERATOR_LESS_THAN, "5", " `name` < ?", []interface{}{"5"}, false},
		{"lte", apipb.CRITERION_OPERATOR_LESS_THAN_OR_EQUAL_TO, "5", " `name` <= ?", []interface{}{"5"}, false},
		{"is_null", apipb.CRITERION_OPERATOR_IS_NULL, "", " `name` IS NULL ", nil, false},
		{"is_not_null", apipb.CRITERION_OPERATOR_IS_NOT_NULL, "", " `name` IS NOT NULL ", nil, false},
		{"like_wraps_wildcard", apipb.CRITERION_OPERATOR_LIKE, "ali", " `name` LIKE ?", []interface{}{"%ali%"}, false},
		{"in_splits_csv", apipb.CRITERION_OPERATOR_IN, "a, b ,c", " `name` IN (?,?,?)", []interface{}{"a", "b", "c"}, false},
		{"not_in_strips_brackets", apipb.CRITERION_OPERATOR_NOT_IN, "[a,b]", " `name` NOT IN (?,?)", []interface{}{"a", "b"}, false},
		{"unsupported_op", apipb.CriterionOperator(999), "x", "", nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sql, params, err := convertCriterionOperator("name", c.op, c.value)
			if c.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.wantSQL, sql)
			require.Equal(t, c.wantParams, params)
		})
	}
}

func TestBuildLabelCriterionSQL(t *testing.T) {
	op := &apipb.CriterionOperation{
		Criterion: []*apipb.Criterion{
			{
				FieldName:  "pipeline_run.metadata.labels.env",
				Operator:   apipb.CRITERION_OPERATOR_EQUAL,
				MatchValue: stringMatchValue(t, "prod"),
			},
		},
	}
	queryStrs, params, err := buildLabelCriterionSQL(op, "pipeline_run")
	require.NoError(t, err)
	require.Len(t, queryStrs, 1)
	require.Equal(t,
		" `uid` in (SELECT `obj_uid` FROM pipeline_run_labels WHERE `key`= ? AND `value` = ? )",
		queryStrs[0],
	)
	require.Equal(t, []interface{}{"env", "prod"}, params)
}

func TestBuildLabelCriterionSQL_SkipsNonLabel(t *testing.T) {
	op := &apipb.CriterionOperation{
		Criterion: []*apipb.Criterion{
			{
				FieldName:  "pipeline_run.state",
				Operator:   apipb.CRITERION_OPERATOR_EQUAL,
				MatchValue: stringMatchValue(t, "RUNNING"),
			},
		},
	}
	queryStrs, params, err := buildLabelCriterionSQL(op, "pipeline_run")
	require.NoError(t, err)
	require.Empty(t, queryStrs)
	require.Empty(t, params)
}

func TestBuildFieldCriterionSQL_MapsBaseField(t *testing.T) {
	op := &apipb.CriterionOperation{
		Criterion: []*apipb.Criterion{
			{
				FieldName:  "pipeline_run.metadata.creation_timestamp",
				Operator:   apipb.CRITERION_OPERATOR_GREATER_THAN,
				MatchValue: stringMatchValue(t, "2026-01-01"),
			},
		},
	}
	queryStrs, params, err := buildFieldCriterionSQL(op, nil)
	require.NoError(t, err)
	require.Equal(t, []string{" `create_time` > ?"}, queryStrs)
	require.Equal(t, []interface{}{"2026-01-01"}, params)
}

func TestBuildFieldCriterionSQL_SkipsLabel(t *testing.T) {
	op := &apipb.CriterionOperation{
		Criterion: []*apipb.Criterion{
			{
				FieldName:  "pipeline_run.metadata.labels.env",
				Operator:   apipb.CRITERION_OPERATOR_EQUAL,
				MatchValue: stringMatchValue(t, "prod"),
			},
		},
	}
	queryStrs, params, err := buildFieldCriterionSQL(op, nil)
	require.NoError(t, err)
	require.Empty(t, queryStrs)
	require.Empty(t, params)
}

func TestBuildFieldCriterionSQL_IndexPathMapValidation(t *testing.T) {
	// When indexPathToKeyMap is non-nil, fields must appear in it (or in
	// baseOrderByFields). The map also rewrites the SQL column name.
	indexPathToKeyMap := map[string]string{
		"spec.framework": "framework_col",
	}

	t.Run("known_field_is_rewritten", func(t *testing.T) {
		op := &apipb.CriterionOperation{
			Criterion: []*apipb.Criterion{
				{
					FieldName:  "model.spec.framework",
					Operator:   apipb.CRITERION_OPERATOR_EQUAL,
					MatchValue: stringMatchValue(t, "tensorflow"),
				},
			},
		}
		queryStrs, _, err := buildFieldCriterionSQL(op, indexPathToKeyMap)
		require.NoError(t, err)
		require.Equal(t, []string{" `framework_col` = ?"}, queryStrs)
	})

	t.Run("unknown_field_is_rejected", func(t *testing.T) {
		op := &apipb.CriterionOperation{
			Criterion: []*apipb.Criterion{
				{
					FieldName:  "model.spec.unknown_field",
					Operator:   apipb.CRITERION_OPERATOR_EQUAL,
					MatchValue: stringMatchValue(t, "x"),
				},
			},
		}
		_, _, err := buildFieldCriterionSQL(op, indexPathToKeyMap)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported field")
	})

	t.Run("base_order_by_fields_still_resolve", func(t *testing.T) {
		// Base fields (creation_timestamp, update_timestamp) are always allowed.
		op := &apipb.CriterionOperation{
			Criterion: []*apipb.Criterion{
				{
					FieldName:  "model.metadata.creation_timestamp",
					Operator:   apipb.CRITERION_OPERATOR_GREATER_THAN,
					MatchValue: stringMatchValue(t, "2026-01-01"),
				},
			},
		}
		queryStrs, _, err := buildFieldCriterionSQL(op, indexPathToKeyMap)
		require.NoError(t, err)
		require.Equal(t, []string{" `create_time` > ?"}, queryStrs)
	})
}

func TestBuildCriterionSQL_AndCombination(t *testing.T) {
	op := &apipb.CriterionOperation{
		LogicalOperator: apipb.LOGICAL_OPERATOR_AND,
		Criterion: []*apipb.Criterion{
			{
				FieldName:  "pipeline_run.state",
				Operator:   apipb.CRITERION_OPERATOR_EQUAL,
				MatchValue: stringMatchValue(t, "RUNNING"),
			},
			{
				FieldName:  "pipeline_run.metadata.labels.env",
				Operator:   apipb.CRITERION_OPERATOR_EQUAL,
				MatchValue: stringMatchValue(t, "prod"),
			},
		},
	}
	sql, params, err := buildCriterionSQL(op, "pipeline_run", nil)
	require.NoError(t, err)
	// Field criteria come first, then label criteria, joined by " AND" (suffix-trim pattern).
	require.Equal(t,
		" `state` = ? AND `uid` in (SELECT `obj_uid` FROM pipeline_run_labels WHERE `key`= ? AND `value` = ? )",
		sql,
	)
	require.Equal(t, []interface{}{"RUNNING", "env", "prod"}, params)
}

func TestBuildCriterionSQL_OrCombination(t *testing.T) {
	op := &apipb.CriterionOperation{
		LogicalOperator: apipb.LOGICAL_OPERATOR_OR,
		Criterion: []*apipb.Criterion{
			{
				FieldName:  "pipeline_run.name",
				Operator:   apipb.CRITERION_OPERATOR_EQUAL,
				MatchValue: stringMatchValue(t, "alice"),
			},
			{
				FieldName:  "pipeline_run.name",
				Operator:   apipb.CRITERION_OPERATOR_EQUAL,
				MatchValue: stringMatchValue(t, "bob"),
			},
		},
	}
	sql, _, err := buildCriterionSQL(op, "pipeline_run", nil)
	require.NoError(t, err)
	require.Equal(t, " `name` = ? OR `name` = ?", sql)
}

func TestBuildCriterionSQL_SubOperations(t *testing.T) {
	op := &apipb.CriterionOperation{
		LogicalOperator: apipb.LOGICAL_OPERATOR_AND,
		Criterion: []*apipb.Criterion{
			{
				FieldName:  "pipeline_run.state",
				Operator:   apipb.CRITERION_OPERATOR_EQUAL,
				MatchValue: stringMatchValue(t, "RUNNING"),
			},
		},
		SubOperations: []*apipb.CriterionOperation{
			{
				LogicalOperator: apipb.LOGICAL_OPERATOR_OR,
				Criterion: []*apipb.Criterion{
					{
						FieldName:  "pipeline_run.name",
						Operator:   apipb.CRITERION_OPERATOR_EQUAL,
						MatchValue: stringMatchValue(t, "alice"),
					},
					{
						FieldName:  "pipeline_run.name",
						Operator:   apipb.CRITERION_OPERATOR_EQUAL,
						MatchValue: stringMatchValue(t, "bob"),
					},
				},
			},
		},
	}
	sql, params, err := buildCriterionSQL(op, "pipeline_run", nil)
	require.NoError(t, err)
	// Sub-operation is wrapped as " (<sub>)" where <sub> has its own leading space.
	require.Equal(t, " `state` = ? AND ( `name` = ? OR `name` = ?)", sql)
	require.Equal(t, []interface{}{"RUNNING", "alice", "bob"}, params)
}

func TestBuildCriterionSQL_NilOperation(t *testing.T) {
	sql, params, err := buildCriterionSQL(nil, "pipeline_run", nil)
	require.NoError(t, err)
	require.Empty(t, sql)
	require.Empty(t, params)
}

func TestBuildOrderBySQL(t *testing.T) {
	cases := []struct {
		name string
		in   []*apipb.OrderBy
		want string
	}{
		{
			name: "empty",
			in:   nil,
			want: "",
		},
		{
			name: "base_field_with_crd_prefix",
			in: []*apipb.OrderBy{
				{Field: "pipeline_run.metadata.creation_timestamp", Dir: apipb.SORT_ORDER_DESC},
			},
			want: " ORDER BY `create_time` DESC",
		},
		{
			name: "regular_column_asc",
			in: []*apipb.OrderBy{
				{Field: "pipeline_run.name", Dir: apipb.SORT_ORDER_ASC},
			},
			want: " ORDER BY `name` ASC",
		},
		{
			name: "multi_clause",
			in: []*apipb.OrderBy{
				{Field: "pipeline_run.metadata.creation_timestamp", Dir: apipb.SORT_ORDER_DESC},
				{Field: "pipeline_run.name", Dir: apipb.SORT_ORDER_ASC},
			},
			want: " ORDER BY `create_time` DESC, `name` ASC",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, buildOrderBySQL(c.in))
		})
	}
}

func TestExtractMatchValue_StringValueWrapper(t *testing.T) {
	v, err := extractMatchValue(stringMatchValue(t, "alice"))
	require.NoError(t, err)
	require.Equal(t, "alice", v)
}

func TestExtractMatchValue_Nil(t *testing.T) {
	_, err := extractMatchValue(nil)
	require.Error(t, err)
}

func TestExtractMatchValue_RawBytesFallback_Sanitized(t *testing.T) {
	// Raw bytes that aren't a StringValue: sanitizeRe strips characters
	// outside [a-zA-Z0-9\-_. ,].
	any := &gogotypes.Any{Value: []byte("alice;DROP TABLE")}
	v, err := extractMatchValue(any)
	require.NoError(t, err)
	require.Equal(t, "aliceDROP TABLE", v)
}

func TestExtractMatchValue_EmptyStringWrapperFallsThrough(t *testing.T) {
	// Matches internal UnmarshalStringValueFromAny: when StringValue unwraps to
	// an empty string, we fall through to the raw-bytes path. The raw bytes of
	// an empty StringValue are also empty, so the result is "".
	any := stringMatchValue(t, "")
	v, err := extractMatchValue(any)
	require.NoError(t, err)
	require.Equal(t, "", v)
}
