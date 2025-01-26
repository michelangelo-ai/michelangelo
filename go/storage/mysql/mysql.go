package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql" // load mysql driver
	pbtypes "github.com/gogo/protobuf/types"
	"github.com/iancoleman/strcase"
	"github.com/michelangelo-ai/michelangelo/go/api"
	apiutil "github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	"github.com/michelangelo-ai/michelangelo/go/storage/object"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	googleProto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	k8s_apiutil "sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)
const (
	configKey = "mysql"
)

// Config defines MySQL configuration.
type Config struct {
	Db             string            `yaml:"database"`
	User           string            `yaml:"user"`
	Password       string            `yaml:"password"`
	Host           string            `yaml:"host"`
	Port           int               `yaml:"port"`
	Params         map[string]string `yaml:"params"`
	MaxConLifetime int               `yaml:"max_con_lifetime_sec"`
	MaxOpenCons    int               `yaml:"max_open_cons"`
	MaxIdleCons    int               `yaml:"max_idle_cons"`
}

type backfillObject struct {
	query  string
	params []interface{}
}

var (
	lock            = &sync.Mutex{}
	metadataStorage *metadataStorageImpl
	sqlOpen         = sql.Open // for unit test
)

// Params are the dependencies to build metadata storage library
type Params struct {
	fx.In

	MySQLCfg      Config
	StorageConfig storage.MetadataStorageConfig
	Logger        *zap.Logger
	ObjectManager object.Manager
	Scope         tally.Scope
}

// GetMetadataStorage returns the metadataStorageImpl singleton.
// Returns nil if MySQL database configuration is not found.
func GetMetadataStorage(params Params) (storage.MetadataStorage, error) {
	lock.Lock()
	defer lock.Unlock()

	if params.StorageConfig.EnableMetadataStorage == false {
		return nil, nil
	}

	if params.MySQLCfg.Db == "" {
		return nil, nil
	}

	if metadataStorage != nil {
		return metadataStorage, nil
	}

	// construct MySQL data source name
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", params.MySQLCfg.User, params.MySQLCfg.Password,
		params.MySQLCfg.Host, params.MySQLCfg.Port, params.MySQLCfg.Db)
	mysqlParams := "parseTime=true"
	for k, v := range params.MySQLCfg.Params {
		mysqlParams += "&"
		mysqlParams += fmt.Sprintf("%s=%s", k, v)
	}
	dsn += "?" + mysqlParams

	params.Logger.Info(fmt.Sprintf("Connecting to MySQL database %s:%d/%s",
		params.MySQLCfg.Host, params.MySQLCfg.Port, params.MySQLCfg.Db))
	// open db connection
	db, err := sqlOpen("mysql", dsn)
	if err != nil {
		params.Logger.Error(fmt.Sprintf("Failed to set up connect to MySQL database: %v", err))
		return nil, err
	}

	params.Logger.Info(fmt.Sprintf("Setting MySQL connection parameters:\n MaxConLifetime:%d sec\n MaxOpenCons:%d\n"+
		" MaxIdleCons%d",
		params.MySQLCfg.MaxConLifetime,
		params.MySQLCfg.MaxOpenCons,
		params.MySQLCfg.MaxIdleCons))
	db.SetConnMaxLifetime(time.Second * time.Duration(params.MySQLCfg.MaxConLifetime))
	db.SetMaxOpenConns(params.MySQLCfg.MaxOpenCons)
	db.SetMaxIdleConns(params.MySQLCfg.MaxIdleCons)

	// verify db connection
	if err = db.Ping(); err != nil {
		params.Logger.Error(fmt.Sprintf("Failed to connect to MySQL database: %v", err))
		return nil, err
	}
	params.Logger.Info("Connected to MySQL database")

	metadataStorage = &metadataStorageImpl{
		db, params.Logger.Sugar(), nil, nil, params.ObjectManager,
		params.Scope.SubScope("mysql_sync"),
	}

	if params.StorageConfig.EnableResourceVersionCache {
		if err = metadataStorage.initResVerCache(); err != nil {
			metadataStorage = nil
			return nil, err
		}
	}

	return metadataStorage, nil
}

type metadataStorageImpl struct {
	db     *sql.DB
	logger *zap.SugaredLogger
	// an in memory cache that contains the (uid, resource version) pairs of all known objects
	resVerCache      map[string]uint64
	resVerCacheMutex *sync.RWMutex
	objManager       object.Manager
	metrics          tally.Scope
}

type listQueryStruct struct {
	labelSelectorWith        string
	labelSelectorJoin        string
	labelSelectorQueryParams []interface{}
	fieldSelectorQuery       string
	fieldSelectorQueryParams []interface{}
	limit                    int64
	offset                   int64
}

// Upsert implements storage.metadataStorageImpl.Upsert()
// Returns nil if successful, otherwise a gRPC status error is returned.
func (m *metadataStorageImpl) Upsert(ctx context.Context, runtimeObj runtime.Object, direct bool,
	indexedFields []storage.IndexedField,
) error {
	accessor, err := meta.Accessor(runtimeObj)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid runtimeObj: %v", err)
	}
	gvk, err := k8s_apiutil.GVKForObject(runtimeObj, m.objManager.Scheme())
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "cannot get groupVersionKind from runtimeObj: %v", err)
	}
	tableName := apiutil.ToSnakeCase(gvk.Kind)
	resVer := accessor.GetResourceVersion()
	resVersion, err := strconv.ParseUint(resVer, 10, 64)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid resource version resVersion %s: %s", resVer, err.Error())
	}
	uid := string(accessor.GetUID())

	if !direct && m.hasResVerCache() {
		// check if object is already synced to MySQL
		if resVersion == m.readResVerCache(uid) {
			// this version has already been synced to MySQL, do not need to sync again
			return nil
		}
	}
	obj := object.MySQLObject{}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to begin MySQL transaction: %v", err)
	}
	// This deferred rollback() will always be called when this function returns.
	// Calling rollback() on a committed transaction has no effect.
	defer rollback(tx)

	if !direct {
		recordSyncDelay(accessor, gvk.Kind, m.metrics, m.logger)

		// convert runtimeObject to MySQL object
		if err = m.objManager.FromRuntimeObject(runtimeObj, &obj); err != nil {
			return status.Errorf(codes.InvalidArgument, err.Error())
		}
		// Upsert main table
		query, queryParams, buildErr := buildIndirectUpsertQuery(tableName, obj, indexedFields)
		if buildErr != nil {
			return status.Errorf(codes.Internal, buildErr.Error())
		}
		err = m.exec(tx, query, queryParams...)
		if err != nil {
			return status.Errorf(codes.Internal, "failed to upsert object to MySQL table %s: %v", tableName, err)
		}

		// Update annotations
		err = m.updateKVPairs(tx, tableName+"_annotations", obj.UID, obj.Annotations)
		if err != nil {
			return err
		}

		// Update labels
		err = m.updateKVPairs(tx, tableName+"_labels", obj.UID, obj.Labels)
		if err != nil {
			return err
		}

	} else {
		query := "SELECT `uid`, `res_version`, `json`, `proto` FROM `" + tableName +
			"` WHERE `namespace`=? AND `name`=? AND `delete_time` IS NULL"
		results, queryErr := tx.Query(query, accessor.GetNamespace(), accessor.GetName())
		if queryErr != nil {
			return status.Errorf(codes.Internal, queryErr.Error())
		}
		defer closeResults(results)

		if !results.Next() {
			return status.Errorf(codes.NotFound, "update failed: cannot find the specified object in mysql: [uid: %v,"+
				" namespace: %v, name: %v]", accessor.GetUID(), accessor.GetNamespace(), accessor.GetName())
		}

		// retrieve the existing data from mysql
		err = results.Scan(&obj.UID, &obj.ResourceVersion, &obj.JSON, &obj.Proto)
		if err != nil {
			return status.Errorf(codes.Internal, err.Error())
		}
		closeResults(results)

		if obj.ResourceVersion != resVersion {
			return status.Errorf(codes.InvalidArgument,
				"the object has been modified; please apply your changes to the latest version and try again")
		}
		if obj.UID != uid {
			return status.Errorf(codes.InvalidArgument, "UID in requirement: %s, UID in object meta: %s", uid, obj.UID)
		}

		// update resource version
		obj.ResourceVersion++
		if obj.ResourceVersion < MinMySQLResVer {
			obj.ResourceVersion = MinMySQLResVer
		}
		// update annotations and labels
		obj.Annotations = accessor.GetAnnotations()
		obj.Labels = accessor.GetLabels()
		obj.UpdateTimestamp = time.Now()

		newRuntimeObj := reflect.New(reflect.TypeOf(runtimeObj).Elem()).Interface().(runtime.Object)
		// convert the updated object to runtime object
		if err = m.objManager.ToRuntimeObject(obj, newRuntimeObj); err != nil {
			return status.Errorf(codes.Internal, err.Error())
		}
		newAccessor, accessErr := meta.Accessor(newRuntimeObj)
		if accessErr != nil {
			return accessErr
		}
		// update annotations, labels
		newAccessor.SetLabels(obj.Labels)
		newAccessor.SetAnnotations(obj.Annotations)

		// serialize the updated runtime object to obj
		if err = m.objManager.FromRuntimeObject(newRuntimeObj, &obj); err != nil {
			return status.Errorf(codes.Internal, err.Error())
		}

		// Update annotations
		err = m.updateKVPairs(tx, tableName+"_annotations", uid, obj.Annotations)
		if err != nil {
			return err
		}

		// Update labels
		err = m.updateKVPairs(tx, tableName+"_labels", uid, obj.Labels)
		if err != nil {
			return err
		}

		// Write to mysql
		query = "UPDATE `" + tableName + "` SET `json`=?, `proto`=?, `update_time`=?, `res_version`=? WHERE `uid`=?"
		err = m.exec(tx, query, obj.JSON, obj.Proto, obj.UpdateTimestamp, obj.ResourceVersion, obj.UID)
		if err != nil {
			return status.Errorf(codes.Internal, err.Error())
		}
	}
	if err = tx.Commit(); err != nil {
		return status.Errorf(codes.Internal, err.Error())
	}
	if m.hasResVerCache() {
		m.writeResVerCache(uid, obj.ResourceVersion)
	}
	if direct {
		err = m.objManager.ToRuntimeObject(obj, runtimeObj)
		if err != nil {
			return status.Errorf(codes.Internal, err.Error())
		}
	}

	return nil
}

// GetByName implements storage.metadataStorageImpl.GetByName()
// Returns nil if successful, otherwise a gRPC status error is returned.
func (m *metadataStorageImpl) GetByName(ctx context.Context, namespace string, name string,
	runtimeObj runtime.Object,
) error {
	where := "`namespace`=\"" + namespace + "\" AND `name`=\"" + name + "\""
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return status.Errorf(codes.Internal, err.Error())
	}
	return m.get(tx, runtimeObj, where)
}

// GetByID implements storage.metadataStorageImpl.GetByID()
// Returns nil if successful, otherwise a gRPC status error is returned.
func (m *metadataStorageImpl) GetByID(ctx context.Context, uid string, runtimeObj runtime.Object) error {
	where := "`uid`=\"" + uid + "\""
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return status.Errorf(codes.Internal, err.Error())
	}
	return m.get(tx, runtimeObj, where)
}

func (m *metadataStorageImpl) get(tx *sql.Tx, runtimeObj runtime.Object, where string) (err error) {
	startTime := time.Now()
	obj := object.MySQLObject{}
	if err = m.objManager.FromRuntimeObject(runtimeObj, &obj); err != nil {
		return status.Errorf(codes.InvalidArgument, err.Error())
	}
	defer emitMySQLMetics(m.metrics, time.Now().Sub(startTime), obj.Kind, "Get", obj.Namespace, &err)

	tableName := apiutil.ToSnakeCase(obj.Kind)

	// This deferred rollback() will always be called when this function returns.
	// Calling rollback() on a committed transaction has no effect.
	defer rollback(tx)

	query := "SELECT `uid`, `group_ver`, `namespace`, `name`, `res_version`, `create_time`, `update_time`, " +
		"`proto` FROM `" + tableName + "` WHERE " + where + " AND `delete_time` IS NULL"
	results, err := tx.Query(query)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to retrieve object from MySQL table %s: %v", tableName, err)
	}
	defer closeResults(results)

	if !results.Next() {
		return status.Errorf(codes.NotFound, "%s %s not found", obj.Kind, where)
	}

	err = results.Scan(&obj.UID, &obj.GroupVer, &obj.Namespace, &obj.Name, &obj.ResourceVersion,
		&obj.CreationTimestamp, &obj.UpdateTimestamp, &obj.Proto)
	if err != nil {
		return status.Errorf(codes.Internal, err.Error())
	}
	closeResults(results)

	if err = tx.Commit(); err != nil {
		return status.Errorf(codes.Internal, err.Error())
	}
	if err = m.objManager.ToRuntimeObject(obj, runtimeObj); err != nil {
		return status.Errorf(codes.Internal, err.Error())
	}

	return nil
}

func (m *metadataStorageImpl) Backfill(ctx context.Context, createFn storage.PrepareBackfillParams,
	options storage.BackfillOptions,
) (endTime *time.Time, err error) {
	if options.BatchSize <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "invalid batch size")
	}

	typeMeta := v1.TypeMeta{
		Kind:       options.Kind,
		APIVersion: "michelangelo.uber.com/v2",
	}
	tableName := apiutil.ToSnakeCase(typeMeta.Kind)

	query := "SELECT `uid`, `group_ver`, `namespace`, `name`, `res_version`, `create_time`, `json`, `proto` FROM `" +
		tableName + "`"

	whereClause, queryParams := prepareWhereClause(options)
	query += whereClause + " ORDER BY `create_time` ASC"

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	// This deferred rollback() will always be called when this function returns.
	// Calling rollback() on a committed transaction has no effect.
	defer rollback(tx)

	results, err := tx.Query(query, queryParams...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list object to MySQL table %s: %v", tableName, err)
	}
	defer closeResults(results)

	tmpObject, err := m.objManager.Scheme().New(typeMeta.GroupVersionKind())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get runtime object from typeMeta, check if "+
			"typeMeta is registered. typeMeta: %v", typeMeta)
	}

	var backfillObjects []backfillObject
	processedCount := 0
	processedCreationTime := time.Time{}

	for results.Next() {
		obj := object.MySQLObject{}
		err = results.Scan(&obj.UID, &obj.GroupVer, &obj.Namespace, &obj.Name, &obj.ResourceVersion,
			&obj.CreationTimestamp, &obj.JSON, &obj.Proto)
		if err != nil {
			return nil, status.Errorf(codes.Internal, fmt.Sprintf("failed in Scan: %s", err.Error()))
		}

		// pagination invariant
		if processedCount >= options.BatchSize && obj.CreationTimestamp.After(processedCreationTime) {
			break
		}
		processedCount++
		processedCreationTime = obj.CreationTimestamp

		newObject := tmpObject.DeepCopyObject()
		if err = m.objManager.ToRuntimeObject(obj, newObject); err != nil {
			m.logger.Errorf("failed to convert object from JSON to Go object, uid: %v, namespace: %v, name: %v",
				obj.UID, obj.Namespace, obj.Name)
			return nil, status.Errorf(codes.Internal, fmt.Sprintf("failed in ToRuntimeObject: %s", err.Error()))
		}

		backfillParam, createErr := createFn(newObject)
		if backfillParam == nil {
			continue
		}
		if createErr != nil {
			return &processedCreationTime, status.Errorf(codes.Internal, fmt.Sprintf("failed in createFn: %s",
				createErr.Error()))
		}

		if err = m.objManager.FromRuntimeObject(backfillParam.Object, &obj); err != nil {
			return &processedCreationTime, status.Errorf(codes.InvalidArgument,
				fmt.Sprintf("failed in FromRuntimeObjecg: %s", err.Error()))
		}
		updateQuery, updateParams, buildErr := buildIndirectUpsertQuery(tableName, obj, backfillParam.IndexedFields)
		if buildErr != nil {
			return &processedCreationTime, status.Errorf(codes.Internal,
				fmt.Sprintf("failed in buildIndirectUpsertQuery: %s", buildErr.Error()))
		}

		backfillObjects = append(backfillObjects, backfillObject{
			query:  updateQuery,
			params: updateParams,
		})
	}

	closeResults(results)

	for _, backfillObj := range backfillObjects {
		err = m.exec(tx, backfillObj.query, backfillObj.params...)
		if err != nil {
			m.logger.Errorf("failed to update a row in backfill. query: %v, params: %v",
				backfillObj.query, backfillObj.params)
			return &processedCreationTime, status.Errorf(codes.Internal, "failed to backfill table %s: error: %s",
				tableName, err.Error())
		}
	}

	if err = tx.Commit(); err != nil {
		return &processedCreationTime, status.Errorf(codes.Internal, fmt.Sprintf("failed in Commit: %s",
			err.Error()))
	}

	if processedCount >= options.BatchSize {
		return &processedCreationTime, nil
	}

	return nil, nil
}

// List implements storage.metadataStorageImpl.List()
// Returns nil if successful, otherwise a gRPC status error is returned.
func (m *metadataStorageImpl) List(ctx context.Context, typeMeta *v1.TypeMeta, namespace string, opts *v1.ListOptions,
	listOptionsExt *apipb.ListOptionsExt, listResp *storage.ListResponse,
) (err error) {
	startTime := time.Now()
	defer emitMySQLMetics(m.metrics, time.Now().Sub(startTime), typeMeta.Kind, "List", namespace, &err)
	logger := m.logger.With("list-request", startTime.UnixNano()).With("namespace", namespace)
	logger.Info(fmt.Sprintf("start list namespace: %s, options: %v, optionsExt %v", namespace, opts, listOptionsExt))
	tableName := apiutil.ToSnakeCase(typeMeta.Kind)

	var queryStruct *listQueryStruct

	indexPathToKeyMap, err := m.objManager.GetIndexPathToKeyMap(typeMeta.GroupVersionKind())
	if err != nil {
		return err
	}

	if listOptionsExt != nil && listOptionsExt.Operation != nil {
		queryStruct, err = buildQueryFromListOptExtV2(listOptionsExt.GetOperation(), typeMeta, indexPathToKeyMap)
		if err != nil {
			return err
		}
		if listOptionsExt.Pagination != nil {
			queryStruct.limit = int64(listOptionsExt.Pagination.Limit)
			queryStruct.offset = int64(listOptionsExt.Pagination.Offset)
		}
		if queryStruct.fieldSelectorQuery != "" {
			queryStruct.fieldSelectorQuery = " AND (" + queryStruct.fieldSelectorQuery
			queryStruct.fieldSelectorQuery += " )"
		}
	} else {
		queryStruct, err = buildQueryFromListOpt(opts, tableName, indexPathToKeyMap)
		if err != nil {
			return err
		}
	}

	var queryParams []interface{}
	query := queryStruct.labelSelectorWith + "SELECT `uid`, `group_ver`, `namespace`, `name`, `res_version`, " +
		"`create_time`, `update_time`, `proto` FROM `" + tableName + "` " + queryStruct.
		labelSelectorJoin + " WHERE "
	if namespace != "" {
		query += "`namespace`=? AND `delete_time` IS NULL"
		queryParams = append(queryParams, namespace)
	} else {
		query += "`delete_time` IS NULL"
	}

	if queryStruct.fieldSelectorQuery != "" {
		query += queryStruct.fieldSelectorQuery
		queryParams = append(queryParams, queryStruct.fieldSelectorQueryParams...)
	}

	if len(queryStruct.labelSelectorQueryParams) != 0 {
		queryParams = append(queryStruct.labelSelectorQueryParams, queryParams...)
	}

	// order by
	if listOptionsExt != nil {
		orderByQueryStr, err := buildOrderByQuery(listOptionsExt.OrderBy, indexPathToKeyMap)
		if err != nil {
			return err
		}
		query += orderByQueryStr
	}

	if queryStruct.limit > 0 {
		query += " LIMIT ?"
		queryParams = append(queryParams, queryStruct.limit)

		if queryStruct.offset > 0 {
			query += " OFFSET ?"
			queryParams = append(queryParams, queryStruct.offset)
		}
	}

	if err := m.executeListQueryAndProcessResult(ctx, logger, query, queryParams, queryStruct.limit, queryStruct.offset, typeMeta, listResp); err != nil {
		return err
	}

	endTime := time.Now()
	logger.Info(fmt.Sprintf("list %d items took %v", len(listResp.Items), endTime.Sub(startTime)))

	return nil
}

// Delete implements storage.metadataStorageImpl.Delete()
// Returns nil if successful, otherwise a gRPC status error is returned.
func (m *metadataStorageImpl) Delete(ctx context.Context, typeMeta *v1.TypeMeta, namespace string, name string) error {
	tableName := apiutil.ToSnakeCase(typeMeta.Kind)

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return status.Errorf(codes.Internal, err.Error())
	}
	// This deferred rollback() will always be called when this function returns.
	// Calling rollback() on a committed transaction has no effect.
	defer rollback(tx)

	query := "SELECT `uid` FROM `" + tableName + "` WHERE `namespace`=? AND `name`=? AND `delete_time` IS NULL"
	results, err := tx.Query(query, namespace, name)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to retrieve object UID from MySQL table %s: %v", tableName, err)
	}
	defer closeResults(results)

	if !results.Next() {
		return status.Errorf(codes.NotFound, "%s `namespace`=\"%s\" AND `name`=\"%s\" not found", typeMeta.Kind,
			namespace, name)
	}
	var uid string
	err = results.Scan(&uid)
	if err != nil {
		return status.Errorf(codes.Internal, err.Error())
	}
	closeResults(results)

	query = "UPDATE `" + tableName + "` SET `delete_time`=? WHERE `uid`=?"
	err = m.exec(tx, query, time.Now(), uid) // TODO(yingz): sync delete timestamp from k8s/etcd
	if err != nil {
		return status.Errorf(codes.Internal, "failed to delete object from MySQL table %s: %v", tableName, err)
	}

	if err = tx.Commit(); err != nil {
		return status.Errorf(codes.Internal, err.Error())
	}

	return nil
}

// DeleteCollection implements storage.metadataStorageImpl.DeleteCollection()
// Returns nil if successful, otherwise a gRPC status error is returned.
func (m *metadataStorageImpl) DeleteCollection(_ context.Context, _ string, _ *v1.DeleteOptions,
	_ *v1.ListOptions,
) error {
	return status.Errorf(codes.Unimplemented, "MySQL list is not implemented yet")
}

func (m *metadataStorageImpl) QueryByTemplateID(ctx context.Context, typeMeta *v1.TypeMeta, templateID string,
	listOptionsExt *apipb.ListOptionsExt, listResp *storage.ListResponse,
) error {
	startTime := time.Now()
	logger := m.logger.With("query-template-request", startTime.UnixNano())
	if _, ok := lineageFuncMap[templateID]; !ok {
		return status.Errorf(codes.Internal, "template id: %s does not exist ", templateID)
	}
	query, queryParams, err := lineageFuncMap[templateID](listOptionsExt)
	if err != nil {
		return err
	}

	indexPathToKeyMap, err := m.objManager.GetIndexPathToKeyMap(typeMeta.GroupVersionKind())
	if err != nil {
		return err
	}

	// order by
	if listOptionsExt != nil {
		orderByQueryStr, err := buildOrderByQuery(listOptionsExt.OrderBy, indexPathToKeyMap)
		if err != nil {
			return err
		}
		query += orderByQueryStr
	}

	var limit, offset int64
	if listOptionsExt.Pagination != nil {
		limit = int64(listOptionsExt.Pagination.Limit)
		offset = int64(listOptionsExt.Pagination.Offset)
	}
	// Currently we limit the query result rows to 100,000 to ensure performance.
	// This number is in line with the support from Object Search query.
	// We will lift the limit once we shift the query to mysql slave nodes in the future
	if limit > 0 && limit < 100000 {
		query += " LIMIT ? "
		queryParams = append(queryParams, limit)
		if offset > 0 {
			query += " OFFSET ?"
			queryParams = append(queryParams, offset)
		}
	} else {
		query += " LIMIT 100000 "
	}

	if err := m.executeListQueryAndProcessResult(ctx, logger, query, queryParams, limit, offset, typeMeta, listResp); err != nil {
		return err
	}
	endTime := time.Now()
	logger.Info(fmt.Sprintf("list lineage query %d items took %v", len(listResp.Items), endTime.Sub(startTime)))
	return nil
}

// Close DB connection
func (m *metadataStorageImpl) Close() {
	if m.db != nil {
		// ignore error
		_ = m.db.Close()
	}
}

func (m *metadataStorageImpl) initResVerCache() error {
	m.resVerCache = make(map[string]uint64)
	for typeName := range v2pb.CrdObjects {
		err := func() error {
			tableName := apiutil.ToSnakeCase(typeName)

			m.logger.Info(fmt.Sprintf("Loading resource versions of existing %s objects in MySQL", tableName))

			query := "SELECT `uid`, `res_version` FROM `" + tableName + "` WHERE `delete_time` IS NULL"
			m.logger.Info(query)
			results, err := m.db.Query(query)
			if err != nil {
				return status.Errorf(codes.Internal,
					"failed to retrieve object resource versions from MySQL table %s: %v", tableName, err)
			}
			defer closeResults(results)

			for results.Next() {
				var uid string
				var resVer uint64
				if err = results.Scan(&uid, &resVer); err != nil {
					return status.Errorf(codes.Internal, err.Error())
				}
				m.resVerCache[uid] = resVer
			}

			closeResults(results)
			return nil
		}()
		if err != nil {
			return err
		}
	}
	m.resVerCacheMutex = &sync.RWMutex{}
	return nil
}

func (m *metadataStorageImpl) readResVerCache(uid string) uint64 {
	m.resVerCacheMutex.RLock()
	defer m.resVerCacheMutex.RUnlock()
	return m.resVerCache[uid]
}

func (m *metadataStorageImpl) writeResVerCache(uid string, resVer uint64) {
	m.resVerCacheMutex.Lock()
	defer m.resVerCacheMutex.Unlock()
	m.resVerCache[uid] = resVer
}

func (m *metadataStorageImpl) hasResVerCache() bool {
	return m.resVerCache != nil
}

func (m *metadataStorageImpl) exec(tx *sql.Tx, query string, args ...interface{}) error {
	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer closeStmt(stmt)
	_, err = stmt.Exec(args...)
	return err
}

// updateKVPairs updates annotations or labels of an object.
func (m *metadataStorageImpl) updateKVPairs(tx *sql.Tx, table string, uid string, kv map[string]string) error {
	// delete existing k-v pairs
	query := "DELETE FROM `" + table + "` WHERE `obj_uid`=?"
	err := m.exec(tx, query, uid)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to delete k-v pairs from MySQL table %s: %v", table, err)
	}

	if len(kv) == 0 {
		return nil
	}

	// insert k-v pairs
	query = "INSERT INTO `" + table + "`(`obj_uid`,`key`,`value`) VALUES "
	var values []interface{}
	for k, v := range kv {
		values = append(values, uid)
		values = append(values, k)
		values = append(values, v)
	}
	query += strings.TrimSuffix(strings.Repeat("(?,?,?),", len(kv)), ",")
	err = m.exec(tx, query, values...)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to insert k-v pairs into MySQL table %s: %v", table, err)
	}
	return nil
}

func (m *metadataStorageImpl) getRelationID(tx *sql.Tx, subjectType, predicate, objectType string) (int32,
	error,
) {
	query := "SELECT `id` FROM `relation` WHERE `subject_type` = ? AND `predicate` = ? AND `object_type` = ?"
	results, err := tx.Query(query, subjectType, predicate, objectType)
	defer closeResults(results)
	if err != nil {
		return -1, err
	}
	if !results.Next() {
		return -1, status.Errorf(codes.NotFound, "cannot find the relation [%s, %s, %s]", subjectType, predicate,
			objectType)
	}
	var relationID int32
	err = results.Scan(&relationID)
	if err != nil {
		return -1, err
	}
	closeResults(results)
	return relationID, nil
}

func (m *metadataStorageImpl) executeListQueryAndProcessResult(ctx context.Context, logger *zap.SugaredLogger, query string, queryParams []interface{}, limit int64, offset int64, typeMeta *v1.TypeMeta, listResp *storage.ListResponse) error {
	logger.Info("begin transaction")
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return status.Errorf(codes.Internal, err.Error())
	}
	// This deferred rollback() will always be called when this function returns.
	// Calling rollback() on a committed transaction has no effect.
	defer rollback(tx)

	logger.Info(fmt.Sprintf("SQL query: %s; params: %v", query, queryParams))

	logger.Info("query main table")
	results, err := tx.Query(query, queryParams...)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to query %s to MySQL table: %v", query, err)
	}
	defer closeResults(results)
	tmpObject, err := m.objManager.Scheme().New(typeMeta.GroupVersionKind())
	if err != nil {
		return status.Errorf(codes.Internal, "failed to get runtime object from typeMeta, "+
			"check if typeMeta is registered")
	}

	var objUIDs []string
	for results.Next() {
		obj := object.MySQLObject{}
		err = results.Scan(&obj.UID, &obj.GroupVer, &obj.Namespace, &obj.Name, &obj.ResourceVersion,
			&obj.CreationTimestamp, &obj.UpdateTimestamp, &obj.Proto)
		if err != nil {
			return status.Errorf(codes.Internal, err.Error())
		}

		newObject := tmpObject.DeepCopyObject()
		if err = m.objManager.ToRuntimeObject(obj, newObject); err != nil {
			return status.Errorf(codes.Internal, err.Error())
		}
		listResp.Items = append(listResp.Items, newObject)
		objUIDs = append(objUIDs, obj.UID)
	}

	closeResults(results)
	if err = tx.Commit(); err != nil {
		return status.Errorf(codes.Internal, err.Error())
	}

	if limit > 0 && int64(len(listResp.Items)) >= limit {
		offset += limit
		listResp.Continue = strconv.FormatInt(offset, 10)
	}
	return nil
}

func toTableAlias(idx int) string {
	return fmt.Sprintf("%c", int('A')+idx-1)
}

// This function builds a SQL upsert query for the indirect case (upsert via ingester) .
// The returned error code is nil if successful, otherwise a gRPC status error is returned.
func buildIndirectUpsertQuery(tableName string, obj object.MySQLObject, indexedFields []storage.IndexedField) (string,
	[]interface{},
	error,
) {
	query := "INSERT INTO `" + tableName + "`"
	queryParams := []interface{}{
		obj.UID, obj.GroupVer, obj.Namespace, obj.Name, obj.ResourceVersion,
		obj.CreationTimestamp, time.Now(), obj.JSON, obj.Proto,
	}

	queryFields := "(`uid`,`group_ver`,`namespace`,`name`,`res_version`,`create_time`,`update_time`,`json`,`proto`"
	queryValues := "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?"
	queryDupkeyUpdate := "ON DUPLICATE KEY UPDATE `group_ver`=VALUES(`group_ver`), `namespace`=VALUES(`namespace`), " +
		"`name`=VALUES(`name`), `res_version`=VALUES(`res_version`), `create_time`=VALUES(`create_time`), " +
		"`update_time`=VALUES(`update_time`), `json`=VALUES(`json`), `proto`=VALUES(`proto`)"

	for _, field := range indexedFields {
		if field.Value != nil && reflect.DeepEqual(reflect.TypeOf(field.Value), reflect.TypeOf(&pbtypes.Timestamp{})) {
			field.Value = time.Unix(field.Value.(*pbtypes.Timestamp).GetSeconds(),
				int64(field.Value.(*pbtypes.Timestamp).GetNanos())).UTC()
		}
		if field.Value != nil && reflect.DeepEqual(reflect.TypeOf(field.Value), reflect.TypeOf(&v1.Time{})) {
			t := field.Value.(*v1.Time)
			if t != nil {
				field.Value = t.UTC()
			}
		}
		queryFields += ",`" + field.Key + "`"
		queryValues += ", ?"
		queryDupkeyUpdate += ", `" + field.Key + "`=VALUES(`" + field.Key + "`)"
		// TODO: how should we handle the nil case?
		queryParams = append(queryParams, field.Value)
	}
	query += queryFields + ") " + queryValues + ") " + queryDupkeyUpdate

	return query, queryParams, nil
}

// This function parses the supported selector string into a list of requirements.
// The returned error code is nil if successful, otherwise a gRPC status error is returned.
func parseSelector(selectorStr string, selectorType string) (labels.Requirements, bool, error) {
	if selectorType != "label" && selectorType != "field" {
		return nil, false, status.Errorf(codes.Unimplemented,
			"unsupported selector type. selector: %v, type: %v", selectorStr, selectorType)
	}

	selector, err := labels.Parse(selectorStr)
	if err != nil {
		return nil, false, status.Errorf(codes.InvalidArgument,
			"failed to parse selector. selector: %v, type: %v, err: %v", selector, selectorType, err.Error())
	}

	requirements, selectable := selector.Requirements()
	// check if all requirement are supported
	for _, requirement := range requirements {
		op := requirement.Operator()
		if selectorType == "label" {
			if op == selection.Equals || op == selection.DoubleEquals ||
				op == selection.Exists || op == selection.In {
				continue
			}
		} else if selectorType == "field" {
			if op == selection.Equals || op == selection.DoubleEquals || op == selection.In {
				continue
			}
		}
		return nil, false, status.Errorf(codes.Unimplemented, "unsupported selector operator %v. selector: %v, "+
			"type: %v", requirement.Operator(), selectorStr, selectorType)
	}

	if len(requirements) > 26 {
		return nil, false, status.Errorf(codes.InvalidArgument,
			"too many selector operators, the max number of operators supported is 26. selector: %v", selectorStr)
	}

	return requirements, selectable, nil
}

// This function builds a SQL query string, and a list of query parameters for the field selector.
// The returned error code is nil if successful, otherwise a gRPC status error is returned.
func buildFieldSelectorQuery(fieldSelectorStr string, indexPathToKeyMap map[string]string) (string, []interface{},
	error,
) {
	requirements, selectable, err := parseSelector(fieldSelectorStr, "field")
	if err != nil || selectable == false || len(requirements) == 0 {
		return "", nil, err
	}

	queryWhereStr := ""
	var queryParams []interface{}

	for _, requirement := range requirements {
		key, found := indexPathToKeyMap[requirement.Key()]
		if !found {
			return "", nil, status.Errorf(codes.InvalidArgument,
				"invalid field selector, unsupported field. field: %v", requirement.Key())
		}

		switch requirement.Operator() {
		case selection.Equals, selection.DoubleEquals:
			queryParam := requirement.Values().List()[0]
			if queryParam != "" {
				queryWhereStr += " AND `" + key + "`=?"
				queryParams = append(queryParams, queryParam)
			} else {
				queryWhereStr += " AND (`" + key + "` IS NULL OR `" + key + "`='')"
			}
		case selection.In:
			queryWhereStr += " AND `" + key + "` IN ("
			valueList := requirement.Values().List()
			for i, val := range valueList {
				if i != 0 {
					queryWhereStr += ","
				}
				queryWhereStr += "?"
				queryParams = append(queryParams, val)
			}
			queryWhereStr += ")"
		}
	}

	return queryWhereStr, queryParams, nil
}

// This function builds a SQL with clause, a join string, and a list of query parameters for the label selector.
// The returned error code is nil if successful, otherwise a gRPC status error is returned.
func buildLabelSelectorQuery(labelSelectorStr string, tableName string) (string, string, []interface{}, error) {
	requirements, selectable, err := parseSelector(labelSelectorStr, "label")
	if err != nil || selectable == false || len(requirements) == 0 {
		return "", "", nil, err
	}

	labelTable := tableName + "_labels"
	operatorIdx := 1
	with := "WITH "
	join := ""
	var withParams []interface{}

	for _, requirement := range requirements {
		tableAlias := toTableAlias(operatorIdx)
		if operatorIdx != 1 {
			with += ", "
		}
		with += tableAlias + " AS (SELECT `obj_uid` FROM " + labelTable + " WHERE "
		join += " INNER JOIN " + tableAlias + " ON (`uid`=" + tableAlias + ".`obj_uid`)"

		switch requirement.Operator() {
		case selection.Equals, selection.DoubleEquals:
			with += "`key`=? AND `value`=?)"
			withParams = append(withParams, requirement.Key())
			withParams = append(withParams, requirement.Values().List()[0])
		case selection.Exists:
			with += "`key`=?)"
			withParams = append(withParams, requirement.Key())
		case selection.In:
			with += "`key`=? AND `value` IN ("
			withParams = append(withParams, requirement.Key())
			valueList := requirement.Values().List()
			for i, val := range valueList {
				if i != 0 {
					with += ","
				}
				with += "?"
				withParams = append(withParams, val)
			}
			with += "))"
		}
		operatorIdx++
	}

	if with != "" {
		with += " "
	}

	return with, join, withParams, nil
}

// baseOrderByFields are the base indexed fields for every CRD that is not in IndexPathToKeyMap.
var baseOrderByFields = map[string]string{
	"metadata.creation_timestamp": "create_time",
	// update timestamp is not a field in CRD metadata, but we make a special case here for ordering by update time
	"metadata.update_timestamp": "update_time",
}

// This function builds a SQL query string for the orderBy message.
// The returned error code is nil if successful, otherwise a gRPC status error is returned.
func buildOrderByQuery(orderBy []*apipb.OrderBy, indexPathToKeyMap map[string]string) (string, error) {
	if len(orderBy) == 0 {
		return "", nil
	}

	var queryStr string
	first := true

	for _, order := range orderBy {
		columnName := ""
		if name, ok := baseOrderByFields[order.Field]; ok {
			columnName = name
		} else {
			columnName, ok = indexPathToKeyMap[order.Field]
			if !ok {
				return "", status.Errorf(codes.InvalidArgument,
					"invalid OrderBy field (field is not an indexed field). field: %v", order.Field)
			}
		}

		if first == true {
			first = false
			queryStr = " ORDER BY"
		} else {
			queryStr += ","
		}
		switch order.Dir {
		case apipb.SORT_ORDER_ASC:
			queryStr += " `" + columnName + "` ASC"
		case apipb.SORT_ORDER_DESC:
			queryStr += " `" + columnName + "` DESC"
		default:
			return "", status.Errorf(codes.InvalidArgument,
				"unsupported OrderBy direction. field: %v, direction: %v", order.Field, order.Dir)
		}
	}

	return queryStr, nil
}

var reg = regexp.MustCompile("[^a-zA-Z0-9-_. ,]+")

// build sql query from listOptionsExt
func buildQueryFromListOptExt(listOptionsExt *apipb.ListOptionsExt, typeMeta *v1.TypeMeta,
	indexPathToKeyMap map[string]string,
) (*listQueryStruct, error) {
	labelWithStr, labelJoinStr, labelQueryParams, err := processListOptExtLabel(listOptionsExt, typeMeta)
	if err != nil {
		return nil, err
	}

	fieldQueryStr, fieldQueryParams, err := processListOptExtField(listOptionsExt, indexPathToKeyMap)
	if err != nil {
		return nil, err
	}

	queryStruct := &listQueryStruct{
		labelSelectorWith:        labelWithStr,
		labelSelectorJoin:        labelJoinStr,
		labelSelectorQueryParams: labelQueryParams,
		fieldSelectorQuery:       fieldQueryStr,
		fieldSelectorQueryParams: fieldQueryParams,
	}

	if listOptionsExt.Pagination != nil {
		queryStruct.limit = int64(listOptionsExt.Pagination.Limit)
		queryStruct.offset = int64(listOptionsExt.Pagination.Offset)
	}

	return queryStruct, nil
}

// build sql query from listOptionsExt - rather than splitting the label and field selector
// this v2 version will merge the label and field selector into the field selector query
// by replacing the label query With and Join clause with CRD.uid in (label query)
func buildQueryFromListOptExtV2(operation *apipb.CriterionOperation, typeMeta *v1.TypeMeta,
	indexPathToKeyMap map[string]string) (*listQueryStruct, error) {
	logicalOperator := operation.GetLogicalOperator()
	if _, ok := logicalOperatorMap[logicalOperator.String()]; !ok {
		return nil, status.Errorf(codes.Internal, "logical operator %v currently not supported", logicalOperator)
	}

	labelQueryStrs, labelQueryParams, err := processListOptExtLabelV2(operation, typeMeta)
	if err != nil {
		return nil, err
	}

	fieldQueryStrs, fieldQueryParams, err := processListOptExtFieldV2(operation, indexPathToKeyMap)
	if err != nil {
		return nil, err
	}

	logicalOperatorStr := " " + logicalOperatorMap[logicalOperator.String()]
	queryStr := ""
	var queryParams []interface{}

	// merge label and field conditions into field selector query with logical operator
	if fieldQueryParams != nil {
		for _, fieldQuery := range fieldQueryStrs {
			queryStr += fieldQuery + logicalOperatorStr
		}
		queryParams = append(queryParams, fieldQueryParams...)
	}

	if labelQueryParams != nil {
		for _, labelQuery := range labelQueryStrs {
			queryStr += labelQuery + logicalOperatorStr
		}
		queryParams = append(queryParams, labelQueryParams...)
	}

	if operation.SubOperations != nil {
		for _, subOperation := range operation.SubOperations {
			subQueryStruct, err := buildQueryFromListOptExtV2(subOperation, typeMeta, indexPathToKeyMap)
			if err != nil {
				return nil, err
			}
			// subQueryStruct needs to merged into parent query field selector query
			queryStr += " (" + subQueryStruct.fieldSelectorQuery + ")" + logicalOperatorStr
			queryParams = append(queryParams, subQueryStruct.fieldSelectorQueryParams...)
		}
	}

	if queryStr != "" {
		queryStr = strings.TrimSuffix(queryStr, logicalOperatorStr)
	}

	queryStruct := &listQueryStruct{
		fieldSelectorQuery:       queryStr,
		fieldSelectorQueryParams: queryParams,
	}

	return queryStruct, nil
}

// build sql query from listOptionsExt
func buildQueryFromListOpt(opts *v1.ListOptions, tableName string,
	indexPathToKeyMap map[string]string,
) (*listQueryStruct, error) {
	if opts == nil {
		return nil, status.Errorf(codes.Internal, "got nil list options")
	}
	labelSelectorWithStr, labelSelectorJoinStr, labelSelectorWithParams,
		err := buildLabelSelectorQuery(opts.LabelSelector, tableName)
	if err != nil {
		return nil, err
	}

	fieldSelectorQueryStr, fieldSelectorQueryParams, err := buildFieldSelectorQuery(opts.FieldSelector,
		indexPathToKeyMap)
	if err != nil {
		return nil, err
	}

	var offset int64
	if opts.Continue != "" {
		offset, err = strconv.ParseInt(opts.Continue, 10, 64)
		if err != nil || offset <= 0 {
			return nil, status.Errorf(codes.InvalidArgument, "invalid Continue field in ListOpts. Continue = %v",
				opts.Continue)
		}
	}

	queryStruct := &listQueryStruct{
		labelSelectorWith:        labelSelectorWithStr,
		labelSelectorJoin:        labelSelectorJoinStr,
		labelSelectorQueryParams: labelSelectorWithParams,
		fieldSelectorQuery:       fieldSelectorQueryStr,
		fieldSelectorQueryParams: fieldSelectorQueryParams,
		limit:                    opts.Limit,
		offset:                   offset,
	}

	return queryStruct, nil
}

func processListOptExtLabel(listOptionsExt *apipb.ListOptionsExt, typeMeta *v1.TypeMeta) (string, string,
	[]interface{}, error,
) {
	var withQueryStr string
	var joinQueryStr string
	var labelQueryParam []interface{}
	tableName := apiutil.ToSnakeCase(typeMeta.Kind)
	labelTable := tableName + "_labels"
	operatorIdx := 1

	operation := listOptionsExt.GetOperation()
	logicalOperator := operation.GetLogicalOperator()

	for _, item := range operation.GetCriterion() {
		// get and validate field name
		if !isLabelField(item.GetFieldName()) && !isLabelFieldInMetadata(item.GetFieldName()) {
			continue
		}
		fieldName, err := processFieldName(item.GetFieldName())
		if err != nil {
			return "", "", nil, status.Errorf(codes.Internal, "field name invalid %v", err)
		}

		// get and validate field value
		valueStr, err := UnmarshalStringValueFromAny(item.GetMatchValue())
		if err != nil {
			return "", "", nil, status.Errorf(codes.Internal, "field value invalid %v", err)
		}

		// construct query string
		criterionOperator := item.GetOperator()

		tableAlias := toTableAlias(operatorIdx)
		if operatorIdx != 1 {
			withQueryStr += ", "
		}
		if withQueryStr == "" {
			withQueryStr = "WITH "
		}
		withQueryStr += tableAlias + " AS (SELECT `obj_uid` FROM " + labelTable + " WHERE "
		joinQueryStr += " INNER JOIN " + tableAlias + " ON (`uid`=" + tableAlias + ".`obj_uid`)"

		withQueryStr += "`key`=? AND"
		labelQueryParam = append(labelQueryParam, fieldName)

		valueStr, valueParam, err := convertCriterionOperator("value", criterionOperator, valueStr)
		if err != nil {
			return "", "", nil, status.Errorf(codes.Internal, "error converting label value query string: %v", err)
		}
		valueStr = valueStr + " " + logicalOperatorMap[logicalOperator.String()]

		withQueryStr += valueStr
		withQueryStr = strings.TrimSuffix(withQueryStr, logicalOperatorMap[logicalOperator.String()])
		labelQueryParam = append(labelQueryParam, valueParam...)
		withQueryStr += ")"
		operatorIdx++
	}
	withQueryStr += " "

	return withQueryStr, joinQueryStr, labelQueryParam, nil
}

func processListOptExtLabelV2(operation *apipb.CriterionOperation, typeMeta *v1.TypeMeta) ([]string,
	[]interface{}, error,
) {
	var queryStrs []string
	var labelQueryParam []interface{}
	tableName := apiutil.ToSnakeCase(typeMeta.Kind)
	labelTable := tableName + "_labels"

	for _, item := range operation.GetCriterion() {
		var queryStr string
		// get and validate field name
		if !isLabelField(item.GetFieldName()) && !isLabelFieldInMetadata(item.GetFieldName()) {
			continue
		}
		fieldName, err := processFieldName(item.GetFieldName())
		if err != nil {
			return nil, nil, status.Errorf(codes.Internal, "field name invalid %v", err)
		}

		// get and validate field value
		valueStr, err := UnmarshalStringValueFromAny(item.GetMatchValue())
		if err != nil {
			return nil, nil, status.Errorf(codes.Internal, "field value invalid %v", err)
		}

		// construct query string
		criterionOperator := item.GetOperator()

		queryStr += " `uid` in (SELECT `obj_uid` FROM " + labelTable + " WHERE `key`= ? AND"

		labelQueryParam = append(labelQueryParam, fieldName)

		valueStr, valueParam, err := convertCriterionOperator("value", criterionOperator, valueStr)
		if err != nil {
			return nil, nil, status.Errorf(codes.Internal, "error converting label value query string: %v", err)
		}

		queryStr += valueStr
		labelQueryParam = append(labelQueryParam, valueParam...)
		queryStr += " )"
		queryStrs = append(queryStrs, queryStr)
	}

	return queryStrs, labelQueryParam, nil
}

func processListOptExtField(listOptionsExt *apipb.ListOptionsExt, indexPathToKeyMap map[string]string) (string,
	[]interface{}, error,
) {
	fieldQueryStr := ""
	var fieldQueryParams []interface{}
	operation := listOptionsExt.GetOperation()
	logicalOperator := operation.GetLogicalOperator()
	for _, item := range operation.GetCriterion() {
		// get and validate field name
		if isLabelField(item.GetFieldName()) || isLabelFieldInMetadata(item.GetFieldName()) {
			continue
		}
		fieldName, err := processFieldName(item.GetFieldName())
		if err != nil {
			return "", nil, status.Errorf(codes.Internal, "field name invalid %v", err)
		}

		indexedFieldName, found := indexPathToKeyMap[fieldName]
		if found {
			fieldName = indexedFieldName
		} else {
			indexedFieldName, found = baseOrderByFields[fieldName]
			if found {
				fieldName = indexedFieldName
			} else {
				return "", nil, status.Errorf(codes.InvalidArgument,
					"invalid field selector, unsupported field. field: %v", fieldName)
			}
		}

		// get and validate field value
		valueStr, err := UnmarshalStringValueFromAny(item.GetMatchValue())
		if err != nil {
			return "", nil, status.Errorf(codes.Internal, "field value invalid %v", err)
		}

		// construct query string
		operator := item.GetOperator()

		valueStr, valueParam, err := convertCriterionOperator(fieldName, operator, valueStr)
		if err != nil {
			return "", nil, status.Errorf(codes.Internal, "error converting label value query string: %v", err)
		}
		valueStr = valueStr + " " + logicalOperatorMap[logicalOperator.String()]

		fieldQueryStr += valueStr
		fieldQueryParams = append(fieldQueryParams, valueParam...)
	}

	if fieldQueryStr != "" {
		fieldQueryStr = " AND (" + fieldQueryStr
		fieldQueryStr = strings.TrimSuffix(fieldQueryStr, logicalOperatorMap[logicalOperator.String()])
		fieldQueryStr += ")"
	}
	return fieldQueryStr, fieldQueryParams, nil
}

func processListOptExtFieldV2(operation *apipb.CriterionOperation, indexPathToKeyMap map[string]string) ([]string,
	[]interface{}, error,
) {
	var queryStrs []string
	var fieldQueryParams []interface{}
	for _, item := range operation.GetCriterion() {
		// get and validate field name
		if isLabelField(item.GetFieldName()) || isLabelFieldInMetadata(item.GetFieldName()) {
			continue
		}
		fieldName, err := processFieldName(item.GetFieldName())
		if err != nil {
			return nil, nil, status.Errorf(codes.Internal, "field name invalid %v", err)
		}

		indexedFieldName, found := indexPathToKeyMap[fieldName]
		if found {
			fieldName = indexedFieldName
		} else {
			indexedFieldName, found = baseOrderByFields[fieldName]
			if found {
				fieldName = indexedFieldName
			} else {
				return nil, nil, status.Errorf(codes.InvalidArgument,
					"invalid field selector, unsupported field. field: %v", fieldName)
			}
		}

		// get and validate field value
		valueStr, err := UnmarshalStringValueFromAny(item.GetMatchValue())
		if err != nil {
			return nil, nil, status.Errorf(codes.Internal, "field value invalid %v", err)
		}

		// construct query string
		operator := item.GetOperator()
		valueStr, valueParam, err := convertCriterionOperator(fieldName, operator, valueStr)
		if err != nil {
			return nil, nil, status.Errorf(codes.Internal, "error converting label value query string: %v", err)
		}

		queryStrs = append(queryStrs, valueStr)
		fieldQueryParams = append(fieldQueryParams, valueParam...)
	}

	return queryStrs, fieldQueryParams, nil
}

var searchableCRDMap = map[string]bool{
	"Alert":            true,
	"CachedOutput":     true,
	"Dashboard":        true,
	"Deployment":       true,
	"EvaluationReport": true,
	"Feature":          true,
	"FeatureGroup":     true,
	"ImageBuild":       true,
	"Model":            true,
	"ModelFamily":      true,
	"Project":          true,
	"Pipeline":         true,
	"PipelineRun":      true,
	"Revision":         true,
	"TriggerRun":       true,
	"Draft":            true,
	"PromptTemplate":   true,
	"PromptRun":        true,
	"PromptDeployment": true,
	"LineageEvent":     true,
}

func processFieldName(fieldName string) (string, error) {
	fieldItems := strings.Split(fieldName, ".")
	if len(fieldItems) < 2 {
		return "", status.Errorf(codes.Internal, "field name invalid, at least CRD name and field name is required")
	}

	if _, ok := v2pb.CrdObjects[strcase.ToCamel(fieldItems[0])]; !ok {
		return "", status.Errorf(codes.Internal, "table name %v invalid", fieldItems[0])
	}

	if val, ok := searchableCRDMap[strcase.ToCamel(fieldItems[0])]; !ok || !val {
		return "", status.Errorf(codes.Internal, "Searching CRD %v is not currently supported on Gallery 2.0 ",
			fieldItems[0])
	}

	if isLabelField(fieldName) { // e.g. 'pipeline_run.label.michelangelo/SourcePipelineType'
		fieldName = strings.SplitN(fieldName, ".", 3)[2]
	} else if isLabelFieldInMetadata(fieldName) { // e.g. 'pipeline_run.metadata.labels.michelangelo/SourcePipelineType'
		fieldName = strings.SplitN(fieldName, ".", 4)[3]
	} else {
		fieldName = strings.SplitN(fieldName, ".", 2)[1]
	}
	return fieldName, nil
}

func isLabelField(fieldName string) bool {
	fieldItems := strings.Split(fieldName, ".")
	if len(fieldItems) > 2 && strings.Trim(fieldItems[1], " ") == "label" {
		return true
	}
	return false
}

func isLabelFieldInMetadata(fieldName string) bool {
	fieldItems := strings.Split(fieldName, ".")
	if len(fieldItems) > 3 && strings.Trim(fieldItems[1], " ") == "metadata" && strings.Trim(fieldItems[2], " ") == "labels" {
		return true
	}
	return false
}

func rollback(tx *sql.Tx) {
	// ignore error
	_ = tx.Rollback()
}

func closeResults(results *sql.Rows) {
	// ignore error
	_ = results.Close()
}

func closeStmt(stmt *sql.Stmt) {
	// ignore error
	_ = stmt.Close()
}

func recordSyncDelay(obj v1.Object, kind string, metrics tally.Scope, logger *zap.SugaredLogger) {
	now := time.Now()
	lastUpdateTime := obj.GetCreationTimestamp().Time
	labels := obj.GetLabels()
	if labels != nil {
		updateTimestamp, ok := labels[api.UpdateTimestampLabel]
		if ok {
			micro, err := strconv.ParseInt(updateTimestamp, 10, 64)
			if err != nil {
				logger.Errorf("failed to parse timestamp label %s: %s", updateTimestamp, err.Error())
				return
			}
			t := time.UnixMicro(micro)
			if t.After(lastUpdateTime) {
				lastUpdateTime = t
			}
		}
	}
	delay := now.Sub(lastUpdateTime)
	tag := map[string]string{"kind": kind}
	metrics.Tagged(tag).Gauge(_syncDelay).Update(float64(delay))
	metrics.Tagged(tag).Histogram(_syncDelayHistogram, timespanBuckets).RecordDuration(delay)
}

func prepareWhereClause(options storage.BackfillOptions) (string, []interface{}) {
	var queryParams []interface{}
	whereClause := " WHERE true"
	if !options.StartTime.IsZero() {
		whereClause += " AND `create_time`>?"
		queryParams = append(queryParams, options.StartTime)
	}
	if options.ExcludeDeleted {
		whereClause += " AND `delete_time` IS NULL"
	}
	var ins []string
	if options.NameSpaces != nil && len(options.NameSpaces) > 0 {
		for _, namespace := range options.NameSpaces {
			queryParams = append(queryParams, namespace)
			ins = append(ins, "?")
		}

		whereClause += fmt.Sprintf(" AND `namespace` IN (%s)", strings.Join(ins, ","))
	}

	return whereClause, queryParams
}

func emitMySQLMetics(metrics tally.Scope, latency time.Duration, kind string, operation string, namespace string, err *error) {
	tag := map[string]string{
		"kind":      kind,
		"operation": operation,
		"namespace": namespace,
	}

	metrics.Tagged(tag).Counter(_mysqlQueryCount).Inc(1)
	if *err != nil {
		metrics.Tagged(tag).Counter(_mysqlQueryFailure).Inc(1)
	} else {
		metrics.Tagged(tag).Counter(_mysqlQuerySuccess).Inc(1)
		metrics.Tagged(tag).Histogram(_mysqlQueryLatencyHistogram, timespanBuckets).RecordDuration(latency)
	}
}

// UnmarshalStringValueFromAny unmarshals the value from the given Any object.
func UnmarshalStringValueFromAny(any *pbtypes.Any) (string, error) {
	if any == nil {
		return "", status.Errorf(codes.Internal, "field value is nil")
	}

	// First attempt to decode the value as wrapperspb.StringValue, if it passes, return the value.
	var stringValuePb wrapperspb.StringValue
	if err := googleProto.Unmarshal(any.Value, &stringValuePb); err == nil && stringValuePb.Value != "" {
		return stringValuePb.Value, nil
	}

	// Fallback to directly converting the value to string.
	valueStrRaw := string(any.Value)
	return reg.ReplaceAllString(valueStrRaw, ""), nil
}
