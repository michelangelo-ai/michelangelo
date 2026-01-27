package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	"google.golang.org/protobuf/proto"
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

	tableName := getTableName(object)
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
	tableName := getTableName(object)
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
	tableName := getTableName(object)
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

	query := fmt.Sprintf("SELECT proto FROM %s WHERE delete_time IS NULL", tableName)
	args := []interface{}{}

	if namespace != "" {
		query += " AND namespace = ?"
		args = append(args, namespace)
	}

	// Add label selector if provided
	if listOptions != nil && listOptions.LabelSelector != "" {
		// TODO: Implement proper label selector parsing and SQL generation
		// For now, this is a simplified version
	}

	// Add ordering
	query += " ORDER BY create_time DESC"

	// Add limit if specified
	if listOptions != nil && listOptions.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, listOptions.Limit)
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
	// TODO: Implement based on list options and label selectors
	return fmt.Errorf("DeleteCollection not yet implemented")
}

// QueryByTemplateID queries objects with a predefined query template
func (m *mysqlMetadataStorage) QueryByTemplateID(ctx context.Context, typeMeta *metav1.TypeMeta, templateID string, listOptionsExt *apipb.ListOptionsExt, listResponse *storage.ListResponse) error {
	// TODO: Implement template-based queries
	return fmt.Errorf("QueryByTemplateID not yet implemented")
}

// Backfill performs backfill operation
func (m *mysqlMetadataStorage) Backfill(ctx context.Context, createFn storage.PrepareBackfillParams, opts storage.BackfillOptions) (endTime *time.Time, err error) {
	// TODO: Implement backfill logic
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
	// TODO: Implement direct update with optimistic concurrency control
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

func getTableName(object runtime.Object) string {
	gvk := object.GetObjectKind().GroupVersionKind()
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
