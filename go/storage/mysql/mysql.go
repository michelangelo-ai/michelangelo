package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	proto "github.com/gogo/protobuf/proto"
	gogotypes "github.com/gogo/protobuf/types"
	api "github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
)

// Config holds MySQL configuration
type Config struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
	// MaxOpenConns is the maximum number of open connections to the database
	MaxOpenConns int `yaml:"maxOpenConns"`
	// MaxIdleConns is the maximum number of connections in the idle connection pool
	MaxIdleConns int `yaml:"maxIdleConns"`
	// ConnMaxLifetime is the maximum amount of time a connection may be reused
	ConnMaxLifetime time.Duration `yaml:"connMaxLifetime"`
}

// mysqlMetadataStorage implements storage.MetadataStorage using MySQL
type mysqlMetadataStorage struct {
	db     *sql.DB
	config Config
	scheme *runtime.Scheme
	// indexPathToKeyMaps optionally constrains which proto field paths can appear
	// in ListOptionsExt criteria, per GVK. The outer key is the GVK; the inner
	// map is path → MySQL column name (e.g. "spec.framework" → "framework").
	//
	// When nil OR no entry exists for a given GVK, the storage is "permissive":
	// any field name passes through to the SQL layer (matches the OSS default).
	// When an entry exists, criteria referencing unknown paths are rejected with
	// codes.InvalidArgument (matches the internal indexPathToKeyMap behavior).
	indexPathToKeyMaps map[schema.GroupVersionKind]map[string]string
}

// NewMetadataStorage creates a new MySQL metadata storage.
//
// indexPathToKeyMaps may be nil (permissive — accept any field name in
// ListOptionsExt). When provided, it constrains the field names allowed per
// GVK. See mysqlMetadataStorage.indexPathToKeyMaps for details.
func NewMetadataStorage(config Config, scheme *runtime.Scheme, indexPathToKeyMaps map[schema.GroupVersionKind]map[string]string) (storage.MetadataStorage, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=UTC",
		config.User, config.Password, config.Host, config.Port, config.Database)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to open database connection: %v", err)
	}

	// Set connection pool settings
	if config.MaxOpenConns > 0 {
		db.SetMaxOpenConns(config.MaxOpenConns)
	} else {
		db.SetMaxOpenConns(25) // Default
	}

	if config.MaxIdleConns > 0 {
		db.SetMaxIdleConns(config.MaxIdleConns)
	} else {
		db.SetMaxIdleConns(5) // Default
	}

	if config.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(config.ConnMaxLifetime)
	} else {
		db.SetConnMaxLifetime(5 * time.Minute) // Default
	}

	// Test the connection
	if err := db.PingContext(context.Background()); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to ping database: %v", err)
	}

	return &mysqlMetadataStorage{
		db:                 db,
		config:             config,
		scheme:             scheme,
		indexPathToKeyMaps: indexPathToKeyMaps,
	}, nil
}

// Upsert adds a new object or updates an existing one
func (m *mysqlMetadataStorage) Upsert(ctx context.Context, object runtime.Object, direct bool, indexedFields []storage.IndexedField) error {
	metaObj, err := getObjectMeta(object)
	if err != nil {
		return err
	}

	tableName := m.getTableName(object)
	if tableName == "" {
		return status.Errorf(codes.InvalidArgument, "unable to determine table name for object type")
	}

	groupVer, err := m.groupVersionForObject(object)
	if err != nil {
		return err
	}

	// Serialize object to protobuf
	protoMsg, ok := object.(proto.Message)
	if !ok {
		return status.Errorf(codes.InvalidArgument, "object does not implement proto.Message")
	}
	protoBytes, err := proto.Marshal(protoMsg)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to marshal object to proto: %v", err)
	}

	// Serialize object to JSON
	jsonBytes, err := json.Marshal(object)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to marshal object to JSON: %v", err)
	}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	if direct {
		// Direct update: only update labels, annotations, and resource version
		// Check resource version for optimistic concurrency control
		return m.directUpdate(ctx, tx, tableName, metaObj, object)
	}

	// Full upsert: update all fields. Compute the stable primary key once so the main
	// object row and its label/annotation child rows are all keyed consistently.
	primaryKey := metadataStoragePrimaryKey(metaObj)

	err = m.fullUpsert(ctx, tx, tableName, groupVer, metaObj, protoBytes, jsonBytes, indexedFields)
	if err != nil {
		return err
	}

	// Upsert labels
	err = m.upsertLabels(ctx, tx, tableName, primaryKey, metaObj.GetLabels())
	if err != nil {
		return err
	}

	// Upsert annotations
	err = m.upsertAnnotations(ctx, tx, tableName, primaryKey, metaObj.GetAnnotations())
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetByName retrieves an object by its namespace and name
func (m *mysqlMetadataStorage) GetByName(ctx context.Context, namespace string, name string, object runtime.Object) error {
	tableName := m.getTableName(object)
	if tableName == "" {
		return status.Errorf(codes.InvalidArgument, "unable to determine table name for object type")
	}

	query := fmt.Sprintf(`
		SELECT proto
		FROM %s
		WHERE namespace = ? AND name = ? AND delete_time IS NULL
		LIMIT 1
	`, tableName)

	var protoBytes []byte
	err := m.db.QueryRowContext(ctx, query, namespace, name).Scan(&protoBytes)
	if err == sql.ErrNoRows {
		return status.Errorf(codes.NotFound, "object not found: %s/%s", namespace, name)
	}
	if err != nil {
		return status.Errorf(codes.Internal, "failed to query object: %v", err)
	}

	// Deserialize protobuf
	protoMsg, ok := object.(proto.Message)
	if !ok {
		return status.Errorf(codes.InvalidArgument, "object does not implement proto.Message")
	}
	if err := proto.Unmarshal(protoBytes, protoMsg); err != nil {
		return status.Errorf(codes.Internal, "failed to unmarshal proto: %v", err)
	}

	return nil
}

// GetByID retrieves an object by its UID
func (m *mysqlMetadataStorage) GetByID(ctx context.Context, uid string, object runtime.Object) error {
	tableName := m.getTableName(object)
	if tableName == "" {
		return status.Errorf(codes.InvalidArgument, "unable to determine table name for object type")
	}

	query := fmt.Sprintf(`
		SELECT proto
		FROM %s
		WHERE uid = ? AND delete_time IS NULL
		LIMIT 1
	`, tableName)

	var protoBytes []byte
	err := m.db.QueryRowContext(ctx, query, uid).Scan(&protoBytes)
	if err == sql.ErrNoRows {
		return status.Errorf(codes.NotFound, "object not found with uid: %s", uid)
	}
	if err != nil {
		return status.Errorf(codes.Internal, "failed to query object: %v", err)
	}

	// Deserialize protobuf
	protoMsg, ok := object.(proto.Message)
	if !ok {
		return status.Errorf(codes.InvalidArgument, "object does not implement proto.Message")
	}
	if err := proto.Unmarshal(protoBytes, protoMsg); err != nil {
		return status.Errorf(codes.Internal, "failed to unmarshal proto: %v", err)
	}

	return nil
}

// List queries the CRD's main table and returns matching objects.
//
// There are two ways callers can express filters:
//
//  1. Structured proto path — listOptionsExt.Operation is a CriterionOperation
//     tree (field/operator/value plus AND/OR composition). This is what the
//     UI / search service / Python client emit. Rich operator set: =, !=, >,
//     >=, <, <=, LIKE, IN, NOT_IN, IS_NULL, IS_NOT_NULL, plus arbitrarily
//     nested sub-operations.
//
//  2. Kubernetes selector strings — listOptions.LabelSelector and
//     listOptions.FieldSelector are the standard k8s.io/apimachinery format
//     ("env=prod,region=us-east", "status.state=RUNNING"). This is what
//     kubectl, controller-runtime, and any K8s client emit. Limited operator
//     set: =, ==, in, exists, !key, notin (labels); =, ==, in (fields).
//
// Both paths are checked: if the proto path is empty we parse the selector
// strings. They are not combined — callers use one or the other.
//
// In addition, OrderBy may reference the special label-value field
// "<crd>.metadata.labels.michelangelo/SpecUpdateTimestamp". When it does, the
// query needs a CTE that exposes that one label's values as a virtual column
// for ORDER BY to bind against. See orderByLabel{Field,Key,Column}.
//
// Generated SQL shape:
//
//	[WITH <label CTEs>[, SpecUpdateTimeStampTable AS (...)] ]
//	SELECT `uid`, `group_ver`, `namespace`, `name`, `res_version`,
//	       `create_time`, `update_time`, `proto`
//	FROM `<table>` [<label-table INNER JOINs>]
//	WHERE `namespace`=? AND `delete_time` IS NULL
//	      [AND (<criterion proto fragment> )]
//	      [<field-selector AND clauses>]
//	      [<label-selector NOT EXISTS clauses>]
//	      [ORDER BY ...]
//	      [LIMIT ? [OFFSET ?]]
func (m *mysqlMetadataStorage) List(ctx context.Context, typeMeta *metav1.TypeMeta, namespace string, listOptions *metav1.ListOptions, listOptionsExt *apipb.ListOptionsExt, listResponse *storage.ListResponse) error {
	tableName := getTableNameFromTypeMeta(typeMeta)
	if tableName == "" {
		return status.Errorf(codes.InvalidArgument, "unable to determine table name for type: %s", typeMeta.Kind)
	}
	indexPathToKeyMap := m.indexPathToKeyMap(typeMeta)

	// Build ORDER BY first because the SpecUpdateTimestamp label-ordering case
	// requires us to add a CTE + JOIN before SELECT — we need to know whether
	// that case applies before we start writing the query.
	orderBySQL := ""
	if listOptionsExt != nil && len(listOptionsExt.OrderBy) > 0 {
		orderBySQL = buildOrderBySQL(listOptionsExt.OrderBy)
	}

	// Selector-string path: when the structured proto path isn't being used,
	// parse the K8s LabelSelector / FieldSelector strings into SQL fragments.
	// labelSelectorPieces accumulates label CTEs/joins (positive matches like
	// env=prod, env in (a,b), exists) and NOT EXISTS clauses (negative
	// matches like !env, env notin (a,b)).
	var labelPieces labelSelectorPieces
	var fieldSelectorWhere string
	var fieldSelectorParams []interface{}
	useSelectorStrings := (listOptionsExt == nil || listOptionsExt.Operation == nil) && listOptions != nil
	if useSelectorStrings {
		var err error
		labelPieces, err = buildLabelSelectorSQL(listOptions.LabelSelector, tableName)
		if err != nil {
			return err
		}
		fieldSelectorWhere, fieldSelectorParams, err = buildFieldSelectorSQL(listOptions.FieldSelector, indexPathToKeyMap)
		if err != nil {
			return err
		}
	}

	// SpecUpdateTimestamp ordering: register the CTE + INNER JOIN that
	// projects the label's `value` column into scope so ORDER BY can bind to
	// SpecUpdateTimeStampTable.`value`. Reuses labelPieces because it lives
	// in the same WITH clause as any selector-string label CTEs.
	if strings.Contains(orderBySQL, orderByLabelColumn) {
		labelPieces.withAliases = append(labelPieces.withAliases,
			"SpecUpdateTimeStampTable AS (SELECT `obj_uid`, `value` FROM `"+tableName+"_labels` WHERE `key` = ?)")
		labelPieces.joinClauses = append(labelPieces.joinClauses,
			" INNER JOIN SpecUpdateTimeStampTable ON (`uid` = SpecUpdateTimeStampTable.`obj_uid`)")
		labelPieces.withParams = append(labelPieces.withParams, orderByLabelKey)
	}

	// Assemble the query. The CTEs (if any) come first as a single
	// comma-separated WITH clause, then SELECT/FROM/JOINs, then WHERE.
	var query strings.Builder
	if len(labelPieces.withAliases) > 0 {
		query.WriteString("WITH ")
		query.WriteString(strings.Join(labelPieces.withAliases, ", "))
		query.WriteByte(' ')
	}
	query.WriteString("SELECT `uid`, `group_ver`, `namespace`, `name`, `res_version`, ")
	query.WriteString("`create_time`, `update_time`, `proto` FROM `")
	query.WriteString(tableName)
	query.WriteByte('`')
	for _, j := range labelPieces.joinClauses {
		query.WriteString(j)
	}
	query.WriteString(" WHERE ")

	args := []interface{}{}
	if namespace != "" {
		query.WriteString("`namespace`=? AND `delete_time` IS NULL")
		args = append(args, namespace)
	} else {
		query.WriteString("`delete_time` IS NULL")
	}

	// Structured proto path: append the rendered criterion tree.
	if listOptionsExt != nil && listOptionsExt.Operation != nil {
		criterionSQL, criterionArgs, err := buildCriterionSQL(listOptionsExt.Operation, tableName, indexPathToKeyMap)
		if err != nil {
			return status.Errorf(codes.Internal, "failed to build criterion SQL: %v", err)
		}
		if criterionSQL != "" {
			query.WriteString(" AND (")
			query.WriteString(criterionSQL)
			query.WriteString(" )")
			args = append(args, criterionArgs...)
		}
	}

	// Selector-string path (continued): field-selector AND clauses, then
	// label-selector NOT EXISTS clauses for the negative-match operators
	// (!key, notin) which can't be expressed via the CTE/JOIN scaffold.
	if fieldSelectorWhere != "" {
		query.WriteString(fieldSelectorWhere)
		args = append(args, fieldSelectorParams...)
	}
	for _, w := range labelPieces.whereClauses {
		query.WriteString(w)
	}
	args = append(args, labelPieces.whereParams...)

	query.WriteString(orderBySQL)

	// Pagination: prefer the structured Pagination field; otherwise fall back
	// to ListOptions.Limit + ListOptions.Continue (where Continue is the
	// stringified offset returned by the previous page's listResponse.Continue).
	var limit, offset int64
	if listOptionsExt != nil && listOptionsExt.Pagination != nil {
		limit = int64(listOptionsExt.Pagination.Limit)
		offset = int64(listOptionsExt.Pagination.Offset)
	} else if listOptions != nil && listOptions.Limit > 0 {
		limit = listOptions.Limit
		if listOptions.Continue != "" {
			parsed, err := strconv.ParseInt(listOptions.Continue, 10, 64)
			if err != nil || parsed < 0 {
				return status.Errorf(codes.InvalidArgument,
					"invalid Continue field in ListOpts. Continue = %v", listOptions.Continue)
			}
			offset = parsed
		}
	}
	if limit > 0 {
		query.WriteString(" LIMIT ?")
		args = append(args, limit)
		if offset > 0 {
			query.WriteString(" OFFSET ?")
			args = append(args, offset)
		}
	}

	// CTE placeholders bind first — prepend WITH params before the rest.
	if len(labelPieces.withParams) > 0 {
		args = append(labelPieces.withParams, args...)
	}

	return m.executeListQueryAndProcessResult(ctx, query.String(), args, limit, offset, typeMeta, listResponse)
}

// executeListQueryAndProcessResult runs a SELECT query against the main table,
// unmarshals each row's `proto` column into a runtime.Object, and appends to
// listResp.Items. When limit > 0 and the page is full, sets listResp.Continue
// so callers can fetch the next page (matches internal cursor behavior).
//
// For now, the columns other than `proto` (uid, group_ver, namespace, name,
// res_version, create_time, update_time) are scanned but discarded — the proto
// blob already carries all metadata via the embedded ObjectMeta. The internal
// implementation overwrites these fields from the columns to handle the case
// where the column values are more recent than the serialized proto; that
// merge is not yet implemented in OSS. TODO(#1173): merge column-side res_version /
// update_time onto the runtime object.
func (m *mysqlMetadataStorage) executeListQueryAndProcessResult(ctx context.Context, query string, args []interface{}, limit, offset int64, typeMeta *metav1.TypeMeta, listResp *storage.ListResponse) error {
	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to query objects: %v", err)
	}
	defer rows.Close()

	listResp.Items = []runtime.Object{}
	for rows.Next() {
		var (
			uid, groupVer, ns, name string
			resVersion              string
			createTime, updateTime  time.Time
			protoBytes              []byte
		)
		if err := rows.Scan(&uid, &groupVer, &ns, &name, &resVersion, &createTime, &updateTime, &protoBytes); err != nil {
			return status.Errorf(codes.Internal, "failed to scan row: %v", err)
		}

		obj, err := m.createObjectFromTypeMeta(typeMeta)
		if err != nil {
			return err
		}
		protoMsg, ok := obj.(proto.Message)
		if !ok {
			return status.Errorf(codes.InvalidArgument, "object does not implement proto.Message")
		}
		if err := proto.Unmarshal(protoBytes, protoMsg); err != nil {
			return status.Errorf(codes.Internal, "failed to unmarshal proto: %v", err)
		}

		listResp.Items = append(listResp.Items, obj)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if limit > 0 && int64(len(listResp.Items)) >= limit {
		listResp.Continue = strconv.FormatInt(offset+limit, 10)
	}
	return nil
}

var (
	logicalOperatorMap = map[string]string{
		"LOGICAL_OPERATOR_AND": "AND",
		"LOGICAL_OPERATOR_OR":  "OR",
	}

	// baseOrderByFields maps base proto field paths to MySQL column names.
	baseOrderByFields = map[string]string{
		"metadata.creation_timestamp": "create_time",
		"metadata.update_timestamp":   "update_time",
	}

	// orderByLabelField is the (post-CRD-prefix-strip) field path that triggers
	// the ORDER-BY-by-label-value WITH/JOIN hack. Mirrors the internal
	// implementation: only this exact label key is supported, used by MA Studio
	// to sort by spec-update timestamps stored in the labels table.
	orderByLabelField  = "metadata.labels.michelangelo/SpecUpdateTimestamp"
	orderByLabelKey    = "michelangelo/SpecUpdateTimestamp"
	orderByLabelColumn = "SpecUpdateTimeStampTable.`value`"

	// sanitizeRe strips characters that are unsafe in raw Any fallback values.
	sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9\-_. ,]+`)
)

// isLabelField reports whether fieldName is in "<crd>.label.<key>" format.
func isLabelField(fieldName string) bool {
	parts := strings.Split(fieldName, ".")
	return len(parts) > 2 && strings.TrimSpace(parts[1]) == "label"
}

// isLabelFieldInMetadata reports whether fieldName is in "<crd>.metadata.labels.<key>" format.
func isLabelFieldInMetadata(fieldName string) bool {
	parts := strings.Split(fieldName, ".")
	return len(parts) > 3 && strings.TrimSpace(parts[1]) == "metadata" && strings.TrimSpace(parts[2]) == "labels"
}

// processFieldName strips the CRD prefix from a field name.
// "pipeline_run.state" → "state"
// "pipeline_run.label.michelangelo/Foo" → "michelangelo/Foo"
// "pipeline_run.metadata.labels.michelangelo/Foo" → "michelangelo/Foo"
//
// TODO(#1172): validate the CRD prefix against a registry of known + searchable CRDs.
// Two reasons to add this:
//  1. Catch typos at the API boundary. Today an unknown CRD prefix silently
//     becomes a column lookup against the wrong table; the user sees a cryptic
//     MySQL error rather than a clean "unknown CRD" rejection.
//  2. Gate which CRDs are exposed to ad-hoc search. Some kinds (e.g. very large
//     event tables) are too expensive to query by arbitrary fields and should
//     be opt-in. A whitelist makes that boundary explicit and avoids accidental
//     full-table scans from unscoped queries.
//
// Implementing this requires plumbing a registry from the object/scheme layer
// (or the storage constructor) down into this function — not done here to keep
// the change scoped to mysql.go.
func processFieldName(fieldName string) (string, error) {
	if strings.IndexByte(fieldName, '.') < 0 {
		return "", status.Errorf(codes.InvalidArgument, "field name %q invalid: at least <crd>.<field> is required", fieldName)
	}
	if isLabelField(fieldName) {
		return strings.SplitN(fieldName, ".", 3)[2], nil
	}
	if isLabelFieldInMetadata(fieldName) {
		return strings.SplitN(fieldName, ".", 4)[3], nil
	}
	return strings.SplitN(fieldName, ".", 2)[1], nil
}

// convertCriterionOperator builds a SQL fragment for a single field criterion.
// fieldName must already be the bare column name (CRD prefix stripped).
//
// Output format matches the internal storage/pkg/mysql/lineage_util.go exactly:
//   - All fragments start with " " (a leading space) so they can be concatenated
//     with " AND" / " OR" suffixes via the suffix-trim pattern in buildCriterionSQL.
//   - IS NULL / IS NOT NULL fragments end with a trailing space (legacy from the
//     internal map values "IS NULL "/"IS NOT NULL ").
//   - IN / NOT IN list has no spaces between placeholders (e.g. "(?,?,?)").
//
// TODO(#1171): validate fieldName before splicing it into SQL.
//
// Trust model — both this function and the matching internal one assume the
// caller has already vetted fieldName against an allowlist (internal does so
// in processListOptExtFieldV2 via the per-CRD indexPathToKeyMap). When that
// upstream check is bypassed — and in OSS that's the default whenever the
// constructor is called with a nil indexPathToKeyMaps — fieldName is taken
// straight from the caller-supplied CriterionOperation and embedded between
// backticks here. A caller controlling field_name can break out of the
// backtick-quoted identifier and inject SQL (e.g. field_name="x.` UNION
// SELECT … --" survives processFieldName and lands here as `+ "`" + ` UNION
// SELECT … --` `+ "`" + `).
//
// The right long-term fix is for the OSS constructor to require an
// indexPathToKeyMaps and remove the permissive mode. As a defence-in-depth
// stop-gap, we could reject any fieldName containing characters outside
// [a-zA-Z0-9_] right here (cheap, no schema cost). Neither is done yet to
// keep the public surface compatible with the existing caller sites that
// pass nil today.
func convertCriterionOperator(fieldName string, op apipb.CriterionOperator, value string) (string, []interface{}, error) {
	qf := " `" + fieldName + "` "
	switch op {
	case apipb.CRITERION_OPERATOR_IS_NULL:
		return qf + "IS NULL ", nil, nil
	case apipb.CRITERION_OPERATOR_IS_NOT_NULL:
		return qf + "IS NOT NULL ", nil, nil
	case apipb.CRITERION_OPERATOR_EQUAL:
		return qf + "= ?", []interface{}{value}, nil
	case apipb.CRITERION_OPERATOR_NOT_EQUAL:
		return qf + "!= ?", []interface{}{value}, nil
	case apipb.CRITERION_OPERATOR_GREATER_THAN:
		return qf + "> ?", []interface{}{value}, nil
	case apipb.CRITERION_OPERATOR_GREATER_THAN_OR_EQUAL_TO:
		return qf + ">= ?", []interface{}{value}, nil
	case apipb.CRITERION_OPERATOR_LESS_THAN:
		return qf + "< ?", []interface{}{value}, nil
	case apipb.CRITERION_OPERATOR_LESS_THAN_OR_EQUAL_TO:
		return qf + "<= ?", []interface{}{value}, nil
	case apipb.CRITERION_OPERATOR_LIKE:
		return qf + "LIKE ?", []interface{}{"%" + value + "%"}, nil
	case apipb.CRITERION_OPERATOR_IN, apipb.CRITERION_OPERATOR_NOT_IN:
		sqlOp := "IN"
		if op == apipb.CRITERION_OPERATOR_NOT_IN {
			sqlOp = "NOT IN"
		}
		valueList := strings.Split(strings.Trim(value, " [](){}"), ",")
		var sb strings.Builder
		sb.WriteString(qf)
		sb.WriteString(sqlOp)
		sb.WriteString(" (")
		args := make([]interface{}, 0, len(valueList))
		for i, v := range valueList {
			if i != 0 {
				sb.WriteByte(',')
			}
			sb.WriteByte('?')
			args = append(args, strings.Trim(v, " "))
		}
		sb.WriteByte(')')
		return sb.String(), args, nil
	default:
		return "", nil, status.Errorf(codes.InvalidArgument, "operator %v currently not supported", op)
	}
}

// buildLabelCriterionSQL converts label criteria into uid-IN-subquery SQL fragments.
//
// Each fragment matches the internal output exactly:
//
//	" `uid` in (SELECT `obj_uid` FROM <labelTable> WHERE `key`= ? AND `value` = ? )"
//
// Note: lowercase `in`, no backticks around <labelTable>, trailing " )".
func buildLabelCriterionSQL(op *apipb.CriterionOperation, tableName string) ([]string, []interface{}, error) {
	var queryStrs []string
	var params []interface{}
	labelTable := tableName + "_labels"

	for _, item := range op.GetCriterion() {
		if !isLabelField(item.GetFieldName()) && !isLabelFieldInMetadata(item.GetFieldName()) {
			continue
		}
		labelKey, err := processFieldName(item.GetFieldName())
		if err != nil {
			return nil, nil, status.Errorf(codes.InvalidArgument, "label field name invalid: %v", err)
		}

		criterionOp := item.GetOperator()
		var valueStr string
		if !isNoParamOp(criterionOp) {
			valueStr, err = extractMatchValue(item.GetMatchValue())
			if err != nil {
				return nil, nil, status.Errorf(codes.InvalidArgument, "label field value invalid: %v", err)
			}
		}

		valueSQL, valueParams, err := convertCriterionOperator("value", criterionOp, valueStr)
		if err != nil {
			return nil, nil, status.Errorf(codes.InvalidArgument, "error converting label value: %v", err)
		}

		// valueSQL already begins with a leading space (from convertCriterionOperator),
		// so concatenating " AND" + valueSQL yields " AND `value` = ?".
		queryStr := " `uid` in (SELECT `obj_uid` FROM " + labelTable + " WHERE `key`= ? AND" + valueSQL + " )"
		queryStrs = append(queryStrs, queryStr)
		params = append(params, labelKey)
		params = append(params, valueParams...)
	}

	return queryStrs, params, nil
}

// buildFieldCriterionSQL converts non-label criteria into SQL fragments.
// Each fragment begins with a leading space (see convertCriterionOperator).
//
// indexPathToKeyMap (when non-nil) maps proto field paths to MySQL column
// names; criteria referencing paths not in the map are rejected. When nil,
// field names are passed through unchanged after the bare baseOrderByFields
// rewrite (permissive mode).
func buildFieldCriterionSQL(op *apipb.CriterionOperation, indexPathToKeyMap map[string]string) ([]string, []interface{}, error) {
	var queryStrs []string
	var params []interface{}

	for _, item := range op.GetCriterion() {
		if isLabelField(item.GetFieldName()) || isLabelFieldInMetadata(item.GetFieldName()) {
			continue
		}
		fieldName, err := processFieldName(item.GetFieldName())
		if err != nil {
			return nil, nil, status.Errorf(codes.InvalidArgument, "field name invalid: %v", err)
		}

		// Resolve to a column name. Order: per-CRD indexPathToKeyMap, then
		// baseOrderByFields, then permissive passthrough (only when no map).
		if col, ok := indexPathToKeyMap[fieldName]; ok {
			fieldName = col
		} else if col, ok := baseOrderByFields[fieldName]; ok {
			fieldName = col
		} else if indexPathToKeyMap != nil {
			return nil, nil, status.Errorf(codes.InvalidArgument,
				"invalid field selector, unsupported field. field: %v", fieldName)
		}

		criterionOp := item.GetOperator()
		var valueStr string
		if !isNoParamOp(criterionOp) {
			valueStr, err = extractMatchValue(item.GetMatchValue())
			if err != nil {
				return nil, nil, status.Errorf(codes.InvalidArgument, "field value invalid: %v", err)
			}
		}

		queryStr, valueParams, err := convertCriterionOperator(fieldName, criterionOp, valueStr)
		if err != nil {
			return nil, nil, status.Errorf(codes.InvalidArgument, "error converting field criterion: %v", err)
		}

		queryStrs = append(queryStrs, queryStr)
		params = append(params, valueParams...)
	}

	return queryStrs, params, nil
}

// buildCriterionSQL recursively converts a CriterionOperation into a SQL WHERE fragment.
// Output is byte-equivalent to the internal buildQueryFromListOptExtV2:
// - Each fragment is suffixed with the logical operator (" AND" or " OR")
// - The trailing logical-operator suffix is then trimmed
// - Sub-operations are wrapped as " (<sub>)" before being suffixed
//
// All fragments produced by buildFieldCriterionSQL / buildLabelCriterionSQL begin
// with a leading space, so concatenation produces correct spacing.
//
// indexPathToKeyMap is forwarded to buildFieldCriterionSQL — see that function
// for the validation contract.
func buildCriterionSQL(op *apipb.CriterionOperation, tableName string, indexPathToKeyMap map[string]string) (string, []interface{}, error) {
	if op == nil {
		return "", nil, nil
	}

	logicalOp, ok := logicalOperatorMap[op.GetLogicalOperator().String()]
	if !ok {
		return "", nil, status.Errorf(codes.InvalidArgument, "logical operator %v currently not supported", op.GetLogicalOperator())
	}
	logicalOpStr := " " + logicalOp

	fieldQueryStrs, fieldParams, err := buildFieldCriterionSQL(op, indexPathToKeyMap)
	if err != nil {
		return "", nil, err
	}
	labelQueryStrs, labelParams, err := buildLabelCriterionSQL(op, tableName)
	if err != nil {
		return "", nil, err
	}

	var queryStr strings.Builder
	queryParams := make([]interface{}, 0, len(fieldParams)+len(labelParams))

	for _, q := range fieldQueryStrs {
		queryStr.WriteString(q)
		queryStr.WriteString(logicalOpStr)
	}
	queryParams = append(queryParams, fieldParams...)

	for _, q := range labelQueryStrs {
		queryStr.WriteString(q)
		queryStr.WriteString(logicalOpStr)
	}
	queryParams = append(queryParams, labelParams...)

	for _, sub := range op.SubOperations {
		subSQL, subParams, err := buildCriterionSQL(sub, tableName, indexPathToKeyMap)
		if err != nil {
			return "", nil, err
		}
		if subSQL == "" {
			continue
		}
		queryStr.WriteString(" (")
		queryStr.WriteString(subSQL)
		queryStr.WriteString(")")
		queryStr.WriteString(logicalOpStr)
		queryParams = append(queryParams, subParams...)
	}

	return strings.TrimSuffix(queryStr.String(), logicalOpStr), queryParams, nil
}

// isNoParamOp reports whether op needs no match value (IS NULL / IS NOT NULL).
func isNoParamOp(op apipb.CriterionOperator) bool {
	return op == apipb.CRITERION_OPERATOR_IS_NULL || op == apipb.CRITERION_OPERATOR_IS_NOT_NULL
}

// labelSelectorPieces holds the SQL fragments produced from a single
// metav1.ListOptions.LabelSelector string. WITH-clause aliases (and their
// matching JOINs) are emitted for positive-match operators (=, ==, in, exists);
// NOT-EXISTS subqueries in `whereClauses` are emitted for negative-match
// operators (!key, key notin (...)). The two are accumulated separately so
// the caller can intersperse with other CTEs (e.g. orderByLabel hack).
type labelSelectorPieces struct {
	withAliases  []string // each: "<alias> AS (SELECT `obj_uid` FROM `<t>_labels` WHERE ...)"
	joinClauses  []string // each: " INNER JOIN <alias> ON (`uid`=<alias>.`obj_uid`)"
	whereClauses []string // each: " AND NOT EXISTS (...)"
	withParams   []interface{}
	whereParams  []interface{}
}

// parseSelector parses a Kubernetes-style selector string into requirements
// and validates each requirement uses an operator we support.
//
// selectorType ∈ {"label", "field"}:
//   - "label" supports: =, ==, in, exists, !key, notin
//   - "field" supports: =, ==, in (subset — k8s field selectors are stricter)
func parseSelector(selectorStr, selectorType string) (labels.Requirements, bool, error) {
	if selectorType != "label" && selectorType != "field" {
		return nil, false, status.Errorf(codes.Unimplemented,
			"unsupported selector type. selector: %v, type: %v", selectorStr, selectorType)
	}
	selector, err := labels.Parse(selectorStr)
	if err != nil {
		return nil, false, status.Errorf(codes.InvalidArgument,
			"failed to parse selector. selector: %v, type: %v, err: %v", selectorStr, selectorType, err)
	}
	requirements, selectable := selector.Requirements()
	for _, req := range requirements {
		op := req.Operator()
		if selectorType == "label" {
			switch op {
			case selection.Equals, selection.DoubleEquals, selection.Exists,
				selection.In, selection.DoesNotExist, selection.NotIn:
				continue
			}
		} else { // field
			switch op {
			case selection.Equals, selection.DoubleEquals, selection.In:
				continue
			}
		}
		return nil, false, status.Errorf(codes.Unimplemented,
			"unsupported selector operator %v. selector: %v, type: %v",
			req.Operator(), selectorStr, selectorType)
	}
	if len(requirements) > 26 {
		return nil, false, status.Errorf(codes.InvalidArgument,
			"too many selector operators, the max number supported is 26. selector: %v", selectorStr)
	}
	return requirements, selectable, nil
}

// labelAliasName returns the SQL alias for the i-th positive-match label
// requirement (1-indexed). Maps 1→A, 2→B, … 26→Z. Capped by parseSelector's
// 26-requirement limit.
func labelAliasName(i int) string {
	return string(rune('A' + i - 1))
}

// buildLabelSelectorSQL converts a metav1.ListOptions.LabelSelector string
// into label-table CTEs / NOT-EXISTS clauses. Mirrors the internal
// buildLabelSelectorQuery output:
//
//	positive (=, ==, in, exists):
//	    WITH <alias> AS (SELECT `obj_uid` FROM `<t>_labels` WHERE `key`=? [AND `value`...])
//	    + INNER JOIN <alias> ON (`uid`=<alias>.`obj_uid`)
//	negative (!key, key notin (...)):
//	    AND NOT EXISTS (SELECT 1 FROM `<t>_labels` WHERE `obj_uid` = `uid` AND `key` = ? [AND ...])
func buildLabelSelectorSQL(labelSelectorStr, tableName string) (labelSelectorPieces, error) {
	pieces := labelSelectorPieces{}
	if labelSelectorStr == "" {
		return pieces, nil
	}
	requirements, selectable, err := parseSelector(labelSelectorStr, "label")
	if err != nil || !selectable || len(requirements) == 0 {
		return pieces, err
	}
	labelTable := tableName + "_labels"
	posIdx := 0

	for _, req := range requirements {
		switch req.Operator() {
		case selection.DoesNotExist:
			pieces.whereClauses = append(pieces.whereClauses,
				" AND NOT EXISTS (SELECT 1 FROM `"+labelTable+"` WHERE `obj_uid` = `uid` AND `key` = ?)")
			pieces.whereParams = append(pieces.whereParams, req.Key())
			continue
		case selection.NotIn:
			values := req.Values().List()
			if len(values) == 0 {
				continue
			}
			var sb strings.Builder
			sb.WriteString(" AND NOT EXISTS (SELECT 1 FROM `")
			sb.WriteString(labelTable)
			sb.WriteString("` WHERE `obj_uid` = `uid` AND `key` = ? AND `value` IN (")
			pieces.whereParams = append(pieces.whereParams, req.Key())
			for i, v := range values {
				if i != 0 {
					sb.WriteByte(',')
				}
				sb.WriteByte('?')
				pieces.whereParams = append(pieces.whereParams, v)
			}
			sb.WriteString("))")
			pieces.whereClauses = append(pieces.whereClauses, sb.String())
			continue
		}

		// Positive-match operators share a WITH/JOIN scaffold.
		posIdx++
		alias := labelAliasName(posIdx)
		var sb strings.Builder
		sb.WriteString(alias)
		sb.WriteString(" AS (SELECT `obj_uid` FROM `")
		sb.WriteString(labelTable)
		sb.WriteString("` WHERE ")
		switch req.Operator() {
		case selection.Equals, selection.DoubleEquals:
			sb.WriteString("`key`=? AND `value`=?)")
			pieces.withParams = append(pieces.withParams, req.Key(), req.Values().List()[0])
		case selection.Exists:
			sb.WriteString("`key`=?)")
			pieces.withParams = append(pieces.withParams, req.Key())
		case selection.In:
			sb.WriteString("`key`=? AND `value` IN (")
			pieces.withParams = append(pieces.withParams, req.Key())
			values := req.Values().List()
			for i, v := range values {
				if i != 0 {
					sb.WriteByte(',')
				}
				sb.WriteByte('?')
				pieces.withParams = append(pieces.withParams, v)
			}
			sb.WriteString("))")
		}
		pieces.withAliases = append(pieces.withAliases, sb.String())
		pieces.joinClauses = append(pieces.joinClauses,
			" INNER JOIN "+alias+" ON (`uid`="+alias+".`obj_uid`)")
	}
	return pieces, nil
}

// buildFieldSelectorSQL converts a metav1.ListOptions.FieldSelector string
// into AND-prefixed WHERE fragments. Returns the concatenated WHERE fragment
// (including leading " AND" for each clause) and the bind params.
//
// Each requirement's key is looked up in indexPathToKeyMap (the proto path →
// MySQL column map); fields not in the map are rejected with InvalidArgument.
// When indexPathToKeyMap is nil, the path is used as the column name as-is
// (permissive — matches our criterion-builder convention).
func buildFieldSelectorSQL(fieldSelectorStr string, indexPathToKeyMap map[string]string) (string, []interface{}, error) {
	if fieldSelectorStr == "" {
		return "", nil, nil
	}
	requirements, selectable, err := parseSelector(fieldSelectorStr, "field")
	if err != nil || !selectable || len(requirements) == 0 {
		return "", nil, err
	}
	var queryStr strings.Builder
	var params []interface{}
	for _, req := range requirements {
		col := req.Key()
		if mapped, ok := indexPathToKeyMap[col]; ok {
			col = mapped
		} else if indexPathToKeyMap != nil {
			return "", nil, status.Errorf(codes.InvalidArgument,
				"invalid field selector, unsupported field. field: %v", req.Key())
		}
		switch req.Operator() {
		case selection.Equals, selection.DoubleEquals:
			val := req.Values().List()[0]
			if val == "" {
				queryStr.WriteString(" AND (`")
				queryStr.WriteString(col)
				queryStr.WriteString("` IS NULL OR `")
				queryStr.WriteString(col)
				queryStr.WriteString("`='')")
			} else {
				queryStr.WriteString(" AND `")
				queryStr.WriteString(col)
				queryStr.WriteString("`=?")
				params = append(params, val)
			}
		case selection.In:
			queryStr.WriteString(" AND `")
			queryStr.WriteString(col)
			queryStr.WriteString("` IN (")
			values := req.Values().List()
			for i, v := range values {
				if i != 0 {
					queryStr.WriteByte(',')
				}
				queryStr.WriteByte('?')
				params = append(params, v)
			}
			queryStr.WriteByte(')')
		}
	}
	return queryStr.String(), params, nil
}

// buildOrderBySQL builds the ORDER BY clause from a list of OrderBy specs.
//
// Each OrderBy.Field is resolved as follows (matches internal buildOrderByQuery):
//  1. If the bare path matches baseOrderByFields → use the mapped column.
//  2. Else strip the CRD prefix (everything before the first '.'); if the
//     remainder matches baseOrderByFields → use the mapped column.
//  3. Else if the (CRD-stripped) remainder equals orderByLabelField → emit
//     orderByLabelColumn (which is a CTE column reference; the caller is
//     responsible for prepending the matching WITH clause).
//  4. Otherwise the remainder is used as the bare column name.
func buildOrderBySQL(orderBy []*apipb.OrderBy) string {
	if len(orderBy) == 0 {
		return ""
	}
	var clauses []string
	for _, order := range orderBy {
		colName := order.Field
		isLabelValueColumn := false
		if col, ok := baseOrderByFields[colName]; ok {
			colName = col
		} else if idx := strings.IndexByte(colName, '.'); idx >= 0 {
			remainder := colName[idx+1:]
			if col, ok := baseOrderByFields[remainder]; ok {
				colName = col
			} else if remainder == orderByLabelField {
				colName = orderByLabelColumn
				isLabelValueColumn = true
			} else {
				colName = remainder
			}
		}
		dir := "ASC"
		if order.Dir == apipb.SORT_ORDER_DESC {
			dir = "DESC"
		}
		if isLabelValueColumn {
			// orderByLabelColumn already includes its own backticks.
			clauses = append(clauses, colName+" "+dir)
		} else {
			clauses = append(clauses, fmt.Sprintf("`%s` %s", colName, dir))
		}
	}
	return " ORDER BY " + strings.Join(clauses, ", ")
}

// extractMatchValue unpacks a gogo-protobuf types.Any match value into a string.
// Mirrors the internal UnmarshalStringValueFromAny: if the wrapper succeeds AND
// the unwrapped value is non-empty, return it; otherwise fall through to the
// raw bytes (with regex sanitization).
func extractMatchValue(anyVal *gogotypes.Any) (string, error) {
	if anyVal == nil {
		return "", status.Errorf(codes.InvalidArgument, "field value is nil")
	}
	var sv gogotypes.StringValue
	if err := gogotypes.UnmarshalAny(anyVal, &sv); err == nil && sv.Value != "" {
		return sv.Value, nil
	}
	// Fallback: sanitize raw bytes to remove unsafe characters.
	return sanitizeRe.ReplaceAllString(string(anyVal.Value), ""), nil
}

// Delete an object
func (m *mysqlMetadataStorage) Delete(ctx context.Context, typeMeta *metav1.TypeMeta, namespace string, name string) error {
	tableName := getTableNameFromTypeMeta(typeMeta)
	if tableName == "" {
		return status.Errorf(codes.InvalidArgument, "unable to determine table name for type: %s", typeMeta.Kind)
	}

	// Soft delete: set delete_time
	query := fmt.Sprintf(`
		UPDATE %s
		SET delete_time = ?
		WHERE namespace = ? AND name = ? AND delete_time IS NULL
	`, tableName)

	result, err := m.db.ExecContext(ctx, query, time.Now().UTC(), namespace, name)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to delete object: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return status.Errorf(codes.Internal, "failed to get rows affected: %v", err)
	}

	if rowsAffected == 0 {
		return status.Errorf(codes.NotFound, "object not found or already deleted: %s/%s", namespace, name)
	}

	return nil
}

// DeleteCollection deletes a collection of objects
func (m *mysqlMetadataStorage) DeleteCollection(ctx context.Context, namespace string, deleteOptions *metav1.DeleteOptions, listOptions *metav1.ListOptions) error {
	return status.Errorf(codes.Unimplemented, "DeleteCollection not yet implemented")
}

// QueryByTemplateID queries objects with a predefined query template
func (m *mysqlMetadataStorage) QueryByTemplateID(ctx context.Context, typeMeta *metav1.TypeMeta, templateID string, listOptionsExt *apipb.ListOptionsExt, listResponse *storage.ListResponse) error {
	return status.Errorf(codes.Unimplemented, "QueryByTemplateID not yet implemented")
}

// Backfill performs backfill operation
func (m *mysqlMetadataStorage) Backfill(ctx context.Context, createFn storage.PrepareBackfillParams, opts storage.BackfillOptions) (endTime *time.Time, err error) {
	return nil, status.Errorf(codes.Unimplemented, "Backfill not yet implemented")
}

// Close DB connection
func (m *mysqlMetadataStorage) Close() {
	if m.db != nil {
		m.db.Close()
	}
}

// Helper functions

// metadataStoragePrimaryKey returns the stable primary key for metadata storage: the
// MetadataStoragePrimaryKeyAnnotation value when present and non-empty, otherwise the
// object's K8s UID. The main object row and its label/annotation child rows must all be
// keyed by this same value so they remain joinable across cluster migrations, where the
// K8s UID changes but the annotation preserves the original logical identity.
func metadataStoragePrimaryKey(metaObj metav1.Object) string {
	if annotations := metaObj.GetAnnotations(); annotations != nil {
		if pk, ok := annotations[api.MetadataStoragePrimaryKeyAnnotation]; ok && pk != "" {
			return pk
		}
	}
	return string(metaObj.GetUID())
}

func (m *mysqlMetadataStorage) fullUpsert(ctx context.Context, tx *sql.Tx, tableName string, groupVer string, metaObj metav1.Object, protoBytes, jsonBytes []byte, indexedFields []storage.IndexedField) error {
	// Build indexed fields map
	indexedFieldsMap := make(map[string]interface{})
	for _, field := range indexedFields {
		indexedFieldsMap[field.Key] = field.Value
	}

	// Determine primary key: use MetadataStoragePrimaryKeyAnnotation if set, else UID.
	// This enables stable identity across cluster migrations.
	primaryKey := metadataStoragePrimaryKey(metaObj)

	// Build dynamic SQL based on indexed fields
	columns := []string{"uid", "group_ver", "namespace", "name", "res_version", "create_time", "update_time", "proto", "json"}
	placeholders := []string{"?", "?", "?", "?", "?", "?", "?", "?", "?"}
	values := []interface{}{
		primaryKey, // Use annotation-based PK if available
		groupVer,
		metaObj.GetNamespace(),
		metaObj.GetName(),
		metaObj.GetResourceVersion(),
		metaObj.GetCreationTimestamp().Time.UTC(),
		time.Now().UTC(),
		protoBytes,
		jsonBytes,
	}

	// Add indexed fields
	for key, value := range indexedFieldsMap {
		columns = append(columns, key)
		placeholders = append(placeholders, "?")
		values = append(values, value)
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (%s)
		VALUES (%s)
		ON DUPLICATE KEY UPDATE
			res_version = VALUES(res_version),
			update_time = VALUES(update_time),
			proto = VALUES(proto),
			json = VALUES(json)
	`, tableName, strings.Join(columns, ", "), strings.Join(placeholders, ", "))

	// Add indexed fields to UPDATE clause
	for key := range indexedFieldsMap {
		query += fmt.Sprintf(", %s = VALUES(%s)", key, key)
	}

	_, err := tx.ExecContext(ctx, query, values...)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to upsert object: %v", err)
	}

	return nil
}

func (m *mysqlMetadataStorage) directUpdate(ctx context.Context, tx *sql.Tx, tableName string, metaObj metav1.Object, object runtime.Object) error {
	return status.Errorf(codes.Unimplemented, "direct update not yet implemented")
}

func (m *mysqlMetadataStorage) upsertLabels(ctx context.Context, tx *sql.Tx, tableName string, uid string, labels map[string]string) error {
	// Delete existing labels
	deleteQuery := fmt.Sprintf("DELETE FROM %s_labels WHERE obj_uid = ?", tableName)
	if _, err := tx.ExecContext(ctx, deleteQuery, uid); err != nil {
		return status.Errorf(codes.Internal, "failed to delete old labels: %v", err)
	}

	// Insert new labels
	if len(labels) > 0 {
		insertQuery := fmt.Sprintf("INSERT INTO %s_labels (obj_uid, `key`, `value`) VALUES (?, ?, ?)", tableName)
		for key, value := range labels {
			if _, err := tx.ExecContext(ctx, insertQuery, uid, key, value); err != nil {
				return status.Errorf(codes.Internal, "failed to insert label %s=%s: %v", key, value, err)
			}
		}
	}

	return nil
}

func (m *mysqlMetadataStorage) upsertAnnotations(ctx context.Context, tx *sql.Tx, tableName string, uid string, annotations map[string]string) error {
	// Delete existing annotations
	deleteQuery := fmt.Sprintf("DELETE FROM %s_annotations WHERE obj_uid = ?", tableName)
	if _, err := tx.ExecContext(ctx, deleteQuery, uid); err != nil {
		return status.Errorf(codes.Internal, "failed to delete old annotations: %v", err)
	}

	// Insert new annotations
	if len(annotations) > 0 {
		insertQuery := fmt.Sprintf("INSERT INTO %s_annotations (obj_uid, `key`, `value`) VALUES (?, ?, ?)", tableName)
		for key, value := range annotations {
			if _, err := tx.ExecContext(ctx, insertQuery, uid, key, value); err != nil {
				return status.Errorf(codes.Internal, "failed to insert annotation %s=%s: %v", key, value, err)
			}
		}
	}

	return nil
}

func getObjectMeta(object runtime.Object) (metav1.Object, error) {
	metaObj, ok := object.(metav1.Object)
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "object does not implement metav1.Object")
	}
	return metaObj, nil
}

// getTableName returns the snake_case table name for the object's Kind. When
// the object's TypeMeta is empty (a known controller-runtime quirk —
// https://github.com/kubernetes-sigs/controller-runtime/issues/1517), it falls
// back to scheme.ObjectKinds, mirroring the pattern in groupVersionForObject.
func (m *mysqlMetadataStorage) getTableName(object runtime.Object) string {
	gvk := object.GetObjectKind().GroupVersionKind()
	if gvk.Kind == "" && m.scheme != nil {
		if gvks, _, err := m.scheme.ObjectKinds(object); err == nil && len(gvks) > 0 {
			gvk = gvks[0]
		}
	}
	if gvk.Kind == "" {
		return ""
	}
	return utils.ToSnakeCase(gvk.Kind)
}

// indexPathToKeyMap returns the per-GVK field-path → column map used for
// criterion validation, or nil when no map is registered for this typeMeta
// (callers must treat nil as "permissive — accept any field").
func (m *mysqlMetadataStorage) indexPathToKeyMap(typeMeta *metav1.TypeMeta) map[string]string {
	if m.indexPathToKeyMaps == nil || typeMeta == nil {
		return nil
	}
	gv, err := schema.ParseGroupVersion(typeMeta.APIVersion)
	if err != nil {
		return nil
	}
	return m.indexPathToKeyMaps[gv.WithKind(typeMeta.Kind)]
}

func getTableNameFromTypeMeta(typeMeta *metav1.TypeMeta) string {
	if typeMeta == nil || typeMeta.Kind == "" {
		return ""
	}
	return utils.ToSnakeCase(typeMeta.Kind)
}

func (m *mysqlMetadataStorage) createObjectFromTypeMeta(typeMeta *metav1.TypeMeta) (runtime.Object, error) {
	if m.scheme == nil {
		return nil, status.Errorf(codes.InvalidArgument, "scheme is not configured")
	}

	gv, err := schema.ParseGroupVersion(typeMeta.APIVersion)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid apiVersion %q: %v", typeMeta.APIVersion, err)
	}
	gvk := gv.WithKind(typeMeta.Kind)

	obj, err := m.scheme.New(gvk)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to create object for %s: %v", gvk.String(), err)
	}

	return obj, nil
}

func (m *mysqlMetadataStorage) groupVersionForObject(object runtime.Object) (string, error) {
	gvk := object.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		if m.scheme == nil {
			return "", status.Errorf(codes.InvalidArgument, "scheme is not configured to resolve GVK")
		}
		gvks, _, err := m.scheme.ObjectKinds(object)
		if err != nil || len(gvks) == 0 {
			return "", status.Errorf(codes.Internal, "unable to determine GVK for object: %v", err)
		}
		gvk = gvks[0]
	}

	return gvk.GroupVersion().String(), nil
}
