package mysql

import (
	"database/sql/driver"
	"reflect"
	"strings"
	"testing"
	"time"

	gogotypes "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/storage"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// indexTestTime has a fractional second so that truncation is observable.
var indexTestTime = time.Date(2026, 11, 16, 20, 11, 2, 677777253, time.UTC)

func TestToSQLIndexValue(t *testing.T) {
	wantTime := time.Date(2026, 11, 16, 20, 11, 2, 0, time.UTC)

	protoTS, err := gogotypes.TimestampProto(indexTestTime)
	require.NoError(t, err)
	k8sTime := metav1.NewTime(indexTestTime)
	// Same instant expressed in a non-UTC zone; the result must still be UTC.
	k8sTimeNonUTC := metav1.NewTime(indexTestTime.In(time.FixedZone("UTC-8", -8*3600)))

	var nilProtoTS *gogotypes.Timestamp
	var nilK8sTime *metav1.Time

	tests := []struct {
		name string
		in   interface{}
		want interface{}
	}{
		{name: "proto timestamp pointer", in: protoTS, want: wantTime},
		{name: "proto timestamp value", in: *protoTS, want: wantTime},
		{name: "proto timestamp typed nil", in: nilProtoTS, want: nil},
		{name: "proto timestamp epoch is a real value", in: &gogotypes.Timestamp{}, want: time.Unix(0, 0).UTC()},
		{name: "k8s time pointer", in: &k8sTime, want: wantTime},
		{name: "k8s time value", in: k8sTime, want: wantTime},
		{name: "k8s time non-UTC zone is normalized", in: &k8sTimeNonUTC, want: wantTime},
		{name: "k8s time typed nil", in: nilK8sTime, want: nil},
		{name: "k8s time zero value pointer", in: &metav1.Time{}, want: nil},
		{name: "k8s time zero value", in: metav1.Time{}, want: nil},
		{name: "string passes through", in: "PIPELINE_RUN_STATE_SUCCEEDED", want: "PIPELINE_RUN_STATE_SUCCEEDED"},
		{name: "bool passes through", in: true, want: true},
		{name: "int passes through", in: int32(7), want: int32(7)},
		{name: "untyped nil passes through", in: nil, want: nil},
		{name: "time.Time passes through", in: indexTestTime, want: indexTestTime},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toSQLIndexValue(tt.in)
			require.Equal(t, tt.want, got)
			if tm, ok := got.(time.Time); ok {
				require.Equal(t, time.UTC, tm.Location())
			}
			_, err := driver.DefaultParameterConverter.ConvertValue(got)
			require.NoError(t, err, "converted value must be bindable by database/sql")
		})
	}
}

func TestToSQLIndexValue_RawTimestampIsNotBindable(t *testing.T) {
	// Guards the premise of toSQLIndexValue: without conversion, the driver rejects
	// the generated Timestamp index values.
	protoTS, err := gogotypes.TimestampProto(indexTestTime)
	require.NoError(t, err)
	k8sTime := metav1.NewTime(indexTestTime)

	_, err = driver.DefaultParameterConverter.ConvertValue(protoTS)
	require.ErrorContains(t, err, "unsupported type types.Timestamp, a struct")
	_, err = driver.DefaultParameterConverter.ConvertValue(&k8sTime)
	require.ErrorContains(t, err, "unsupported type v1.Time, a struct")
}

// TestIndexedFields_AllKindsBindable is a contract test between the generated
// GetIndexedKeyValuePairs and fullUpsert. For every indexed kind it sets every indexed
// path to a non-zero value, then checks that each indexed value, after
// toSQLIndexValue, is accepted by database/sql. A new index whose Go type the driver
// cannot bind fails here instead of failing every upsert of that kind at runtime.
func TestIndexedFields_AllKindsBindable(t *testing.T) {
	wantTime := indexTestTime.Truncate(time.Second)
	timeColumns := map[string]bool{}

	require.NotEmpty(t, v2pb.IndexesPathToKeyMap)
	for gvk, pathToKey := range v2pb.IndexesPathToKeyMap {
		t.Run(gvk.Kind, func(t *testing.T) {
			prototype, ok := v2pb.CrdObjects[gvk.Kind]
			require.True(t, ok, "no CRD object registered for %s", gvk.Kind)
			obj := reflect.New(reflect.TypeOf(prototype).Elem()).Interface()
			indexed, ok := obj.(storage.IndexedObject)
			require.True(t, ok, "%s does not implement storage.IndexedObject", gvk.Kind)

			for path := range pathToKey {
				setIndexPath(t, reflect.ValueOf(obj).Elem(), path)
			}

			// Kinds with only the default namespace/name indexes return no fields.
			for _, field := range indexed.GetIndexedKeyValuePairs() {
				require.NotNil(t, field.Value, "%s.%s was not populated by the test", gvk.Kind, field.Key)
				value := toSQLIndexValue(field.Value)
				_, err := driver.DefaultParameterConverter.ConvertValue(value)
				require.NoError(t, err, "%s.%s (%T) cannot be bound as a SQL parameter", gvk.Kind, field.Key, field.Value)
				if tm, ok := value.(time.Time); ok {
					require.Equal(t, wantTime, tm, "%s.%s", gvk.Kind, field.Key)
					timeColumns[gvk.Kind+"."+field.Key] = true
				}
			}
		})
	}

	// Make sure the test really exercises the known Timestamp-typed indexes.
	require.True(t, timeColumns["PipelineRun.end_time"], "PipelineRun.end_time was not checked as a time value")
	require.True(t, timeColumns["Deployment.deletion_requested_timestamp"], "Deployment.deletion_requested_timestamp was not checked as a time value")
}

// setIndexPath walks a dotted index path such as "status.end_time" or
// "metadata.deletionTimestamp", allocating nil messages on the way, and sets the leaf
// to a non-zero value. Path segments match the protobuf field name in the struct tag.
func setIndexPath(t *testing.T, v reflect.Value, path string) {
	t.Helper()
	for _, name := range strings.Split(path, ".") {
		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		require.Equal(t, reflect.Struct, v.Kind(), "path %q: cannot descend into %s at %q", path, v.Type(), name)
		v = fieldByProtoName(t, v, name, path)
	}
	setIndexLeaf(t, v, path)
}

func fieldByProtoName(t *testing.T, v reflect.Value, name, path string) reflect.Value {
	t.Helper()
	for i := 0; i < v.NumField(); i++ {
		for _, part := range strings.Split(v.Type().Field(i).Tag.Get("protobuf"), ",") {
			if part == "name="+name {
				return v.Field(i)
			}
		}
	}
	require.Failf(t, "field not found", "path %q: %s has no protobuf field %q", path, v.Type(), name)
	return reflect.Value{}
}

func setIndexLeaf(t *testing.T, v reflect.Value, path string) {
	t.Helper()
	switch v.Type() {
	case reflect.TypeOf(&gogotypes.Timestamp{}):
		ts, err := gogotypes.TimestampProto(indexTestTime)
		require.NoError(t, err)
		v.Set(reflect.ValueOf(ts))
		return
	case reflect.TypeOf(gogotypes.Timestamp{}):
		ts, err := gogotypes.TimestampProto(indexTestTime)
		require.NoError(t, err)
		v.Set(reflect.ValueOf(*ts))
		return
	case reflect.TypeOf(&metav1.Time{}):
		v.Set(reflect.ValueOf(&metav1.Time{Time: indexTestTime}))
		return
	case reflect.TypeOf(metav1.Time{}):
		v.Set(reflect.ValueOf(metav1.Time{Time: indexTestTime}))
		return
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString("contract-test")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1)
	default:
		require.Failf(t, "unhandled index type",
			"path %q has Go type %s; make sure toSQLIndexValue can bind it, then teach setIndexLeaf to populate it",
			path, v.Type())
	}
}
