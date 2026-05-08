package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	proto "github.com/gogo/protobuf/proto"
	gogotypes "github.com/gogo/protobuf/types"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
}

// NewMetadataStorage creates a new MySQL metadata storage
func NewMetadataStorage(config Config, scheme *runtime.Scheme) (storage.MetadataStorage, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=UTC",
		config.User, config.Password, config.Host, config.Port, config.Database)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
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
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &mysqlMetadataStorage{
		db:     db,
		config: config,
		scheme: scheme,
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
		return fmt.Errorf("unable to determine table name for object type")
	}

	groupVer, err := m.groupVersionForObject(object)
	if err != nil {
		return err
	}

	// Serialize object to protobuf
	protoMsg, ok := object.(proto.Message)
	if !ok {
		return fmt.Errorf("object does not implement proto.Message")
	}
	protoBytes, err := proto.Marshal(protoMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal object to proto: %w", err)
	}

	// Serialize object to JSON
	jsonBytes, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object to JSON: %w", err)
	}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if direct {
		// Direct update: only update labels, annotations, and resource version
		// Check resource version for optimistic concurrency control
		return m.directUpdate(ctx, tx, tableName, metaObj, object)
	}

	// Full upsert: update all fields
	err = m.fullUpsert(ctx, tx, tableName, groupVer, metaObj, protoBytes, jsonBytes, indexedFields)
	if err != nil {
		return err
	}

	// Upsert labels
	err = m.upsertLabels(ctx, tx, tableName, string(metaObj.GetUID()), metaObj.GetLabels())
	if err != nil {
		return err
	}

	// Upsert annotations
	err = m.upsertAnnotations(ctx, tx, tableName, string(metaObj.GetUID()), metaObj.GetAnnotations())
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetByName retrieves an object by its namespace and name
func (m *mysqlMetadataStorage) GetByName(ctx context.Context, namespace string, name string, object runtime.Object) error {
	tableName := m.getTableName(object)
	if tableName == "" {
		return fmt.Errorf("unable to determine table name for object type")
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
		return fmt.Errorf("object not found: %s/%s", namespace, name)
	}
	if err != nil {
		return fmt.Errorf("failed to query object: %w", err)
	}

	// Deserialize protobuf
	protoMsg, ok := object.(proto.Message)
	if !ok {
		return fmt.Errorf("object does not implement proto.Message")
	}
	if err := proto.Unmarshal(protoBytes, protoMsg); err != nil {
		return fmt.Errorf("failed to unmarshal proto: %w", err)
	}

	return nil
}

// GetByID retrieves an object by its UID
func (m *mysqlMetadataStorage) GetByID(ctx context.Context, uid string, object runtime.Object) error {
	tableName := m.getTableName(object)
	if tableName == "" {
		return fmt.Errorf("unable to determine table name for object type")
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
		return fmt.Errorf("object not found with uid: %s", uid)
	}
	if err != nil {
		return fmt.Errorf("failed to query object: %w", err)
	}

	// Deserialize protobuf
	protoMsg, ok := object.(proto.Message)
	if !ok {
		return fmt.Errorf("object does not implement proto.Message")
	}
	if err := proto.Unmarshal(protoBytes, protoMsg); err != nil {
		return fmt.Errorf("failed to unmarshal proto: %w", err)
	}

	return nil
}

// List objects
func (m *mysqlMetadataStorage) List(ctx context.Context, typeMeta *metav1.TypeMeta, namespace string, listOptions *metav1.ListOptions, listOptionsExt *apipb.ListOptionsExt, listResponse *storage.ListResponse) error {
	tableName := getTableNameFromTypeMeta(typeMeta)
	if tableName == "" {
		return fmt.Errorf("unable to determine table name for type: %s", typeMeta.Kind)
	}

	query := fmt.Sprintf("SELECT `proto` FROM `%s` WHERE `delete_time` IS NULL", tableName)
	args := []interface{}{}

	if namespace != "" {
		query += " AND `namespace` = ?"
		args = append(args, namespace)
	}

	if listOptionsExt != nil && listOptionsExt.Operation != nil {
		criterionSQL, criterionArgs, err := buildCriterionSQL(listOptionsExt.Operation, tableName)
		if err != nil {
			return fmt.Errorf("failed to build criterion SQL: %w", err)
		}
		if criterionSQL != "" {
			query += " AND (" + criterionSQL + " )"
			args = append(args, criterionArgs...)
		}
	}

	if listOptionsExt != nil && len(listOptionsExt.OrderBy) > 0 {
		query += buildOrderBySQL(listOptionsExt.OrderBy)
	} else {
		query += " ORDER BY `create_time` DESC"
	}

	var limit, offset int64
	if listOptionsExt != nil && listOptionsExt.Pagination != nil {
		limit = int64(listOptionsExt.Pagination.Limit)
		offset = int64(listOptionsExt.Pagination.Offset)
	} else if listOptions != nil && listOptions.Limit > 0 {
		limit = listOptions.Limit
	}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
		if offset > 0 {
			query += " OFFSET ?"
			args = append(args, offset)
		}
	}

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to query objects: %w", err)
	}
	defer rows.Close()

	listResponse.Items = []runtime.Object{}
	for rows.Next() {
		var protoBytes []byte
		if err := rows.Scan(&protoBytes); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}

		// Create new object instance based on type
		obj, err := m.createObjectFromTypeMeta(typeMeta)
		if err != nil {
			return err
		}

		protoMsg, ok := obj.(proto.Message)
		if !ok {
			return fmt.Errorf("object does not implement proto.Message")
		}

		if err := proto.Unmarshal(protoBytes, protoMsg); err != nil {
			return fmt.Errorf("failed to unmarshal proto: %w", err)
		}

		listResponse.Items = append(listResponse.Items, obj)
	}

	return rows.Err()
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
func processFieldName(fieldName string) (string, error) {
	if strings.IndexByte(fieldName, '.') < 0 {
		return "", fmt.Errorf("field name %q invalid: at least <crd>.<field> is required", fieldName)
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
func convertCriterionOperator(fieldName string, op apipb.CriterionOperator, value string) (string, []interface{}, error) {
	qf := "`" + fieldName + "`"
	switch op {
	case apipb.CRITERION_OPERATOR_IS_NULL:
		return qf + " IS NULL", nil, nil
	case apipb.CRITERION_OPERATOR_IS_NOT_NULL:
		return qf + " IS NOT NULL", nil, nil
	case apipb.CRITERION_OPERATOR_EQUAL:
		return qf + " = ?", []interface{}{value}, nil
	case apipb.CRITERION_OPERATOR_NOT_EQUAL:
		return qf + " != ?", []interface{}{value}, nil
	case apipb.CRITERION_OPERATOR_GREATER_THAN:
		return qf + " > ?", []interface{}{value}, nil
	case apipb.CRITERION_OPERATOR_GREATER_THAN_OR_EQUAL_TO:
		return qf + " >= ?", []interface{}{value}, nil
	case apipb.CRITERION_OPERATOR_LESS_THAN:
		return qf + " < ?", []interface{}{value}, nil
	case apipb.CRITERION_OPERATOR_LESS_THAN_OR_EQUAL_TO:
		return qf + " <= ?", []interface{}{value}, nil
	case apipb.CRITERION_OPERATOR_LIKE:
		return qf + " LIKE ?", []interface{}{"%" + value + "%"}, nil
	case apipb.CRITERION_OPERATOR_IN, apipb.CRITERION_OPERATOR_NOT_IN:
		items := strings.Split(strings.Trim(value, " [](){}"), ",")
		placeholders := make([]string, 0, len(items))
		args := make([]interface{}, 0, len(items))
		for _, item := range items {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				placeholders = append(placeholders, "?")
				args = append(args, trimmed)
			}
		}
		if len(placeholders) == 0 {
			return "", nil, fmt.Errorf("field %q: IN/NOT_IN requires at least one value", fieldName)
		}
		sqlOp := "IN"
		if op == apipb.CRITERION_OPERATOR_NOT_IN {
			sqlOp = "NOT IN"
		}
		return fmt.Sprintf("%s %s (%s)", qf, sqlOp, strings.Join(placeholders, ", ")), args, nil
	default:
		return "", nil, fmt.Errorf("unsupported criterion operator: %v", op)
	}
}

// buildLabelCriterionSQL converts label criteria into uid-IN-subquery SQL fragments.
// Each fragment: `uid` IN (SELECT `obj_uid` FROM {table}_labels WHERE `key`=? AND `value`=?)
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
			return nil, nil, fmt.Errorf("label field name invalid: %w", err)
		}

		criterionOp := item.GetOperator()
		var valueStr string
		if criterionOp != apipb.CRITERION_OPERATOR_IS_NULL && criterionOp != apipb.CRITERION_OPERATOR_IS_NOT_NULL {
			valueStr, err = extractMatchValue(item.GetMatchValue())
			if err != nil {
				return nil, nil, fmt.Errorf("label field value invalid: %w", err)
			}
		}

		valueSQL, valueParams, err := convertCriterionOperator("value", criterionOp, valueStr)
		if err != nil {
			return nil, nil, fmt.Errorf("error converting label value: %w", err)
		}

		queryStr := fmt.Sprintf(" `uid` IN (SELECT `obj_uid` FROM `%s` WHERE `key`= ? AND%s)", labelTable, valueSQL)
		queryStrs = append(queryStrs, queryStr)
		params = append(params, labelKey)
		params = append(params, valueParams...)
	}

	return queryStrs, params, nil
}

// buildFieldCriterionSQL converts non-label criteria into SQL fragments.
func buildFieldCriterionSQL(op *apipb.CriterionOperation) ([]string, []interface{}, error) {
	var queryStrs []string
	var params []interface{}

	for _, item := range op.GetCriterion() {
		if isLabelField(item.GetFieldName()) || isLabelFieldInMetadata(item.GetFieldName()) {
			continue
		}
		fieldName, err := processFieldName(item.GetFieldName())
		if err != nil {
			return nil, nil, fmt.Errorf("field name invalid: %w", err)
		}

		// Map base metadata fields to column names.
		if col, ok := baseOrderByFields[fieldName]; ok {
			fieldName = col
		}

		criterionOp := item.GetOperator()
		var valueStr string
		if criterionOp != apipb.CRITERION_OPERATOR_IS_NULL && criterionOp != apipb.CRITERION_OPERATOR_IS_NOT_NULL {
			valueStr, err = extractMatchValue(item.GetMatchValue())
			if err != nil {
				return nil, nil, fmt.Errorf("field value invalid: %w", err)
			}
		}

		queryStr, valueParams, err := convertCriterionOperator(fieldName, criterionOp, valueStr)
		if err != nil {
			return nil, nil, fmt.Errorf("error converting field criterion: %w", err)
		}

		queryStrs = append(queryStrs, queryStr)
		params = append(params, valueParams...)
	}

	return queryStrs, params, nil
}

// buildCriterionSQL recursively converts a CriterionOperation into a SQL WHERE fragment.
// Separates label vs field criteria, appends the logical operator after each fragment,
// then trims the trailing one.
func buildCriterionSQL(op *apipb.CriterionOperation, tableName string) (string, []interface{}, error) {
	if op == nil {
		return "", nil, nil
	}

	logicalOp, ok := logicalOperatorMap[op.GetLogicalOperator().String()]
	if !ok {
		return "", nil, fmt.Errorf("logical operator %v not supported", op.GetLogicalOperator())
	}
	logicalOpStr := " " + logicalOp

	fieldQueryStrs, fieldParams, err := buildFieldCriterionSQL(op)
	if err != nil {
		return "", nil, err
	}

	labelQueryStrs, labelParams, err := buildLabelCriterionSQL(op, tableName)
	if err != nil {
		return "", nil, err
	}

	queryStr := ""
	var queryParams []interface{}

	for _, q := range fieldQueryStrs {
		queryStr += q + logicalOpStr
	}
	queryParams = append(queryParams, fieldParams...)

	for _, q := range labelQueryStrs {
		queryStr += q + logicalOpStr
	}
	queryParams = append(queryParams, labelParams...)

	for _, sub := range op.SubOperations {
		subSQL, subParams, err := buildCriterionSQL(sub, tableName)
		if err != nil {
			return "", nil, err
		}
		if subSQL != "" {
			queryStr += " (" + subSQL + ")" + logicalOpStr
			queryParams = append(queryParams, subParams...)
		}
	}

	if queryStr != "" {
		queryStr = strings.TrimSuffix(queryStr, logicalOpStr)
	}

	return queryStr, queryParams, nil
}

// buildOrderBySQL builds the ORDER BY clause from a list of OrderBy specs.
func buildOrderBySQL(orderBy []*apipb.OrderBy) string {
	if len(orderBy) == 0 {
		return ""
	}
	var clauses []string
	for _, order := range orderBy {
		colName := order.Field
		if col, ok := baseOrderByFields[colName]; ok {
			colName = col
		} else if idx := strings.IndexByte(colName, '.'); idx >= 0 {
			remainder := colName[idx+1:]
			if col, ok := baseOrderByFields[remainder]; ok {
				colName = col
			} else {
				colName = remainder
			}
		}
		dir := "ASC"
		if order.Dir == apipb.SORT_ORDER_DESC {
			dir = "DESC"
		}
		clauses = append(clauses, fmt.Sprintf("`%s` %s", colName, dir))
	}
	return " ORDER BY " + strings.Join(clauses, ", ")
}

// extractMatchValue unpacks a gogo-protobuf types.Any match value into a string.
func extractMatchValue(anyVal *gogotypes.Any) (string, error) {
	if anyVal == nil {
		return "", fmt.Errorf("match_value is nil")
	}
	var sv gogotypes.StringValue
	if err := gogotypes.UnmarshalAny(anyVal, &sv); err == nil {
		return sv.Value, nil
	}
	// Fallback: sanitize raw bytes to remove unsafe characters.
	return sanitizeRe.ReplaceAllString(string(anyVal.Value), ""), nil
}

// Delete an object
func (m *mysqlMetadataStorage) Delete(ctx context.Context, typeMeta *metav1.TypeMeta, namespace string, name string) error {
	tableName := getTableNameFromTypeMeta(typeMeta)
	if tableName == "" {
		return fmt.Errorf("unable to determine table name for type: %s", typeMeta.Kind)
	}

	// Soft delete: set delete_time
	query := fmt.Sprintf(`
		UPDATE %s
		SET delete_time = ?
		WHERE namespace = ? AND name = ? AND delete_time IS NULL
	`, tableName)

	result, err := m.db.ExecContext(ctx, query, time.Now().UTC(), namespace, name)
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("object not found or already deleted: %s/%s", namespace, name)
	}

	return nil
}

// DeleteCollection deletes a collection of objects
func (m *mysqlMetadataStorage) DeleteCollection(ctx context.Context, namespace string, deleteOptions *metav1.DeleteOptions, listOptions *metav1.ListOptions) error {
	return fmt.Errorf("DeleteCollection not yet implemented")
}

// QueryByTemplateID queries objects with a predefined query template
func (m *mysqlMetadataStorage) QueryByTemplateID(ctx context.Context, typeMeta *metav1.TypeMeta, templateID string, listOptionsExt *apipb.ListOptionsExt, listResponse *storage.ListResponse) error {
	return fmt.Errorf("QueryByTemplateID not yet implemented")
}

// Backfill performs backfill operation
func (m *mysqlMetadataStorage) Backfill(ctx context.Context, createFn storage.PrepareBackfillParams, opts storage.BackfillOptions) (endTime *time.Time, err error) {
	return nil, fmt.Errorf("Backfill not yet implemented")
}

// Close DB connection
func (m *mysqlMetadataStorage) Close() {
	if m.db != nil {
		m.db.Close()
	}
}

// Helper functions

func (m *mysqlMetadataStorage) fullUpsert(ctx context.Context, tx *sql.Tx, tableName string, groupVer string, metaObj metav1.Object, protoBytes, jsonBytes []byte, indexedFields []storage.IndexedField) error {
	// Build indexed fields map
	indexedFieldsMap := make(map[string]interface{})
	for _, field := range indexedFields {
		indexedFieldsMap[field.Key] = field.Value
	}

	// Build dynamic SQL based on indexed fields
	columns := []string{"uid", "group_ver", "namespace", "name", "res_version", "create_time", "update_time", "proto", "json"}
	placeholders := []string{"?", "?", "?", "?", "?", "?", "?", "?", "?"}
	values := []interface{}{
		string(metaObj.GetUID()),
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
		return fmt.Errorf("failed to upsert object: %w", err)
	}

	return nil
}

func (m *mysqlMetadataStorage) directUpdate(ctx context.Context, tx *sql.Tx, tableName string, metaObj metav1.Object, object runtime.Object) error {
	return fmt.Errorf("direct update not yet implemented")
}

func (m *mysqlMetadataStorage) upsertLabels(ctx context.Context, tx *sql.Tx, tableName string, uid string, labels map[string]string) error {
	// Delete existing labels
	deleteQuery := fmt.Sprintf("DELETE FROM %s_labels WHERE obj_uid = ?", tableName)
	if _, err := tx.ExecContext(ctx, deleteQuery, uid); err != nil {
		return fmt.Errorf("failed to delete old labels: %w", err)
	}

	// Insert new labels
	if len(labels) > 0 {
		insertQuery := fmt.Sprintf("INSERT INTO %s_labels (obj_uid, `key`, `value`) VALUES (?, ?, ?)", tableName)
		for key, value := range labels {
			if _, err := tx.ExecContext(ctx, insertQuery, uid, key, value); err != nil {
				return fmt.Errorf("failed to insert label %s=%s: %w", key, value, err)
			}
		}
	}

	return nil
}

func (m *mysqlMetadataStorage) upsertAnnotations(ctx context.Context, tx *sql.Tx, tableName string, uid string, annotations map[string]string) error {
	// Delete existing annotations
	deleteQuery := fmt.Sprintf("DELETE FROM %s_annotations WHERE obj_uid = ?", tableName)
	if _, err := tx.ExecContext(ctx, deleteQuery, uid); err != nil {
		return fmt.Errorf("failed to delete old annotations: %w", err)
	}

	// Insert new annotations
	if len(annotations) > 0 {
		insertQuery := fmt.Sprintf("INSERT INTO %s_annotations (obj_uid, `key`, `value`) VALUES (?, ?, ?)", tableName)
		for key, value := range annotations {
			if _, err := tx.ExecContext(ctx, insertQuery, uid, key, value); err != nil {
				return fmt.Errorf("failed to insert annotation %s=%s: %w", key, value, err)
			}
		}
	}

	return nil
}

func getObjectMeta(object runtime.Object) (metav1.Object, error) {
	metaObj, ok := object.(metav1.Object)
	if !ok {
		return nil, fmt.Errorf("object does not implement metav1.Object")
	}
	return metaObj, nil
}

// getTableName returns the lowercased Kind for the object's table. When the
// object's TypeMeta is empty (a known controller-runtime quirk —
// https://github.com/kubernetes-sigs/controller-runtime/issues/1517), it falls
// back to scheme.ObjectKinds, mirroring the pattern in groupVersionForObject.
func (m *mysqlMetadataStorage) getTableName(object runtime.Object) string {
	gvk := object.GetObjectKind().GroupVersionKind()
	if gvk.Kind == "" && m.scheme != nil {
		if gvks, _, err := m.scheme.ObjectKinds(object); err == nil && len(gvks) > 0 {
			gvk = gvks[0]
		}
	}
	return strings.ToLower(gvk.Kind)
}

func getTableNameFromTypeMeta(typeMeta *metav1.TypeMeta) string {
	return strings.ToLower(typeMeta.Kind)
}

func (m *mysqlMetadataStorage) createObjectFromTypeMeta(typeMeta *metav1.TypeMeta) (runtime.Object, error) {
	if m.scheme == nil {
		return nil, fmt.Errorf("scheme is not configured")
	}

	gv, err := schema.ParseGroupVersion(typeMeta.APIVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid apiVersion %q: %w", typeMeta.APIVersion, err)
	}
	gvk := gv.WithKind(typeMeta.Kind)

	obj, err := m.scheme.New(gvk)
	if err != nil {
		return nil, fmt.Errorf("failed to create object for %s: %w", gvk.String(), err)
	}

	return obj, nil
}

func (m *mysqlMetadataStorage) groupVersionForObject(object runtime.Object) (string, error) {
	gvk := object.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		if m.scheme == nil {
			return "", fmt.Errorf("scheme is not configured to resolve GVK")
		}
		gvks, _, err := m.scheme.ObjectKinds(object)
		if err != nil || len(gvks) == 0 {
			return "", fmt.Errorf("unable to determine GVK for object: %w", err)
		}
		gvk = gvks[0]
	}

	return gvk.GroupVersion().String(), nil
}
