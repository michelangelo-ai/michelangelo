package mysql

import (
	"errors"
	"fmt"
	"strings"
	"time"

	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	lineageFuncMap = map[string]func(*apipb.ListOptionsExt) (string, []interface{}, error){
		"FeatureToModelManyToManyQuery":            constructModelFeatureQuery,
		"ActiveModelQuery":                         constructActiveModelQuery,
		"DeployedModelsByTargetTypeQuery":          constructDeployedModelsByTargetTypeQuery,
		"LatestModelCardRevisionForGivenDateRange": constructModelCardRevisionsForGivenDateQuery,
	}
	relationQueryTemplate     = "SELECT `id` FROM `relation` WHERE `subject_type` = ? AND `object_type` = ? AND `predicate` = ? "
	relationshipQueryTemplate = fmt.Sprintf("WITH A AS (SELECT `object_uid` FROM `relationship` WHERE `relation_id` IN (%s"+
		") AND `subject_uid` IN (SELECT `uid` FROM `feature` WHERE `delete_time` IS NULL ", relationQueryTemplate)
	modelLineageQueryTemplate = "SELECT DISTINCT(`model`.`name`), `model`.`uid`, `model`.`group_ver`, `model`.`namespace`, " +
		"`model`.`res_version`, `model`.`create_time`, `model`.`update_time`, `model`.`proto` FROM `model` "
	activeModelTemplate = "SELECT `name`, `uid`, `group_ver`, `namespace`, `res_version`, `create_time`, `update_time`, `proto` FROM `model` "
)

func getFilterMap(listOptionsExt *apipb.ListOptionsExt) map[string]string {
	filters := make(map[string]string)
	if listOptionsExt.Operation != nil {
		for _, item := range listOptionsExt.Operation.Criterion {
			fieldName := item.GetFieldName()
			// get and validate field value
			var valueStr string
			valueStr, err := UnmarshalStringValueFromAny(item.GetMatchValue())
			if err != nil || valueStr == "" {
				continue
			}
			filters[fieldName] = valueStr
		}
	}
	return filters
}

var criterionOperatorSingleParamMap = map[string]string{
	"CRITERION_OPERATOR_EQUAL":                    "=",
	"CRITERION_OPERATOR_NOT_EQUAL":                "!=",
	"CRITERION_OPERATOR_GREATER_THAN":             ">",
	"CRITERION_OPERATOR_GREATER_THAN_OR_EQAUL_TO": ">=",
	"CRITERION_OPERATOR_LESS_THAN":                "<",
	"CRITERION_OPERATOR_LESS_THAN_OR_EQUAL_TO":    "<=",
	"CRITERION_OPERATOR_LIKE":                     "LIKE",
}

var criterionOperatorMultiParamMap = map[string]string{
	"CRITERION_OPERATOR_IN":     "IN",
	"CRITERION_OPERATOR_NOT_IN": "NOT IN",
}

var criterionOperatorNoParamMap = map[string]string{
	"CRITERION_OPERATOR_IS_NULL":     "IS NULL ",
	"CRITERION_OPERATOR_IS_NOT_NULL": "IS NOT NULL ",
}

var logicalOperatorMap = map[string]string{
	"LOGICAL_OPERATOR_AND": "AND",
	"LOGICAL_OPERATOR_OR":  "OR",
}

// construct mysql query string for given filter and operator
func convertCriterionOperator(fieldName string, criterionOperator apipb.CriterionOperator, value string) (string,
	[]interface{}, error,
) {
	var queryParams []interface{}
	var queryStr string

	if _, ok := criterionOperatorSingleParamMap[criterionOperator.String()]; ok {
		queryStr = " `" + fieldName + "` " + criterionOperatorSingleParamMap[criterionOperator.String()] + " ?"
		if criterionOperator == apipb.CRITERION_OPERATOR_LIKE {
			value = "%" + value + "%"
		}
		queryParams = append(queryParams, value)
		return queryStr, queryParams, nil
	}

	if _, ok := criterionOperatorMultiParamMap[criterionOperator.String()]; ok {
		queryStr = " `" + fieldName + "` " + criterionOperatorMultiParamMap[criterionOperator.String()] + " ("
		valueList := strings.Split(strings.Trim(value, " [](){}"), ",")
		for i, val := range valueList {
			if i != 0 {
				queryStr += ","
			}
			queryStr += "?"
			queryParams = append(queryParams, strings.Trim(val, " "))
		}
		queryStr += ")"
		return queryStr, queryParams, nil
	}

	if _, ok := criterionOperatorNoParamMap[criterionOperator.String()]; ok {
		queryStr = " `" + fieldName + "` " + criterionOperatorNoParamMap[criterionOperator.String()]
		return queryStr, queryParams, nil
	}
	return "", nil, status.Errorf(codes.Internal, "operator %v currently not supported", criterionOperator)
}

func constructModelFeatureQuery(listOptionsExt *apipb.ListOptionsExt) (string, []interface{}, error) {
	filters := getFilterMap(listOptionsExt)
	var queryParam []interface{}
	queryParam = append(queryParam, []interface{}{"feature", "model", "spec.feature_store_features"}...)
	query := relationshipQueryTemplate
	if featureName, ok := filters["feature.metadata.name"]; ok {
		query += "AND `name` = ? "
		queryParam = append(queryParam, featureName)
	}
	if featureGroupName, ok := filters["feature.spec.feature_group_name"]; ok {
		query += "AND `feature_group_name` = ? "
		queryParam = append(queryParam, featureGroupName)
	}
	query += ")) "

	// construct model query string
	query += modelLineageQueryTemplate
	query += "INNER JOIN A ON `model`.`uid` = A.`object_uid` "
	if _, ok := filters["deployment.status"]; ok {
		// this filter is to join model with deployment table on model id, because there is no model id column in deployment,
		// we use revision id with the version number truncated to represent model id
		query += "INNER JOIN `deployment` on `model`.`name` = SUBSTRING_INDEX(`deployment`.`current_revision_name`, '-', 4) "
	}
	query += "WHERE `model`.`delete_time` IS NULL "
	if createTime, ok := filters["model.metadata.creation_timestamp"]; ok {
		query += "AND `model`.`create_time` > ? "
		queryParam = append(queryParam, createTime)
	}
	if projectTier, ok := filters["project.spec.tier"]; ok {
		query += "AND `model`.`namespace` IN (SELECT `namespace` FROM `project` WHERE `tier` <= ?) "
		queryParam = append(queryParam, projectTier)
	}
	if _, ok := filters["deployment.status"]; ok {
		query += "AND `deployment`.`delete_time` IS NULL "
	}
	return query, queryParam, nil
}

/*
*
Get all online/offline/both deployed models + models trained over a certain days ago

	    SELECT `name`, `uid`, `group_ver`, `namespace`, `res_version`,
	            `create_time`, `update_time`, `proto`
	    FROM `model`
	    WHERE `model`.`delete_time` IS NULL AND (`model`.`create_time` > ("2024-01-01 00:00:00") OR `model`.`name` IN
		(
			SELECT `SUBSTRING_INDEX(`current_revision_name`, '-', 4)
	        FROM `deployment`
	        WHERE `delete_time` IS NULL
	            AND `state` IN ("DEPLOYMENT_STATE_HEALTHY")
	            AND target_definition_type IN ("TARGET_TYPE_INFERENCE_SERVER")
		))
*/
func constructActiveModelQuery(listOptionsExt *apipb.ListOptionsExt) (string, []interface{}, error) {
	filters := getFilterMap(listOptionsExt)
	var queryParam []interface{}
	query := activeModelTemplate
	query += "WHERE `model`.`delete_time` IS NULL AND ( "
	createTime, ok := filters["model.metadata.creation_timestamp"]
	if !ok {
		return query, queryParam, errors.New("Field: model.metadata.creation_timestamp not provided")
	}

	query += "`model`.`create_time` > ? OR `model`.`name` IN "
	queryParam = append(queryParam, createTime)

	query += "(SELECT SUBSTRING_INDEX(`current_revision_name`, '-', 4) FROM `deployment` WHERE `delete_time` IS NULL"
	deploymentState, ok := filters["deployment.status.state"]
	if !ok {
		return query, queryParam, errors.New("Field: deployment.status.state not provided")
	}
	query += " AND ("
	valueStr, valueParam, err := convertCriterionOperator("state", apipb.CRITERION_OPERATOR_IN, deploymentState)
	valueStr = valueStr + " " + logicalOperatorMap[apipb.LOGICAL_OPERATOR_AND.String()]
	if err != nil {
		return query, queryParam, status.Errorf(codes.Internal, "error converting deploymentState value query string: %v", err)
	}
	query += valueStr
	queryParam = append(queryParam, valueParam...)

	targetTypeList, ok := filters["deployment.spec.definition.type"]
	if !ok {
		return query, queryParam, errors.New("Field: deployment.spec.definition.type not provided")
	}
	valueStr, valueParam, err = convertCriterionOperator("target_definition_type", apipb.CRITERION_OPERATOR_IN, targetTypeList)
	valueStr = valueStr + " " + logicalOperatorMap[apipb.LOGICAL_OPERATOR_AND.String()]
	if err != nil {
		return query, queryParam, status.Errorf(codes.Internal, "error converting target_definition_type value query string: %v", err)
	}
	query += valueStr
	query = strings.TrimSuffix(query, "AND")
	query += ")"
	queryParam = append(queryParam, valueParam...)
	query += "))"

	return query, queryParam, nil
}

/*
*
Get online/offline/both deployed models from a given model list with tm model id

		WITH A AS (
	        SELECT `current_revision_name`
	        FROM `deployment`
	        WHERE `delete_time` IS NULL
	            AND `state` IN ("DEPLOYMENT_STATE_HEALTHY")
	            AND target_definition_type IN ("TARGET_TYPE_OFFLINE"))
	    SELECT `name`, `uid`, `group_ver`, `namespace`, `res_version`,
	            `create_time`, `update_time`, `proto`
	    FROM `model` INNER JOIN A ON `model`.`name` = SUBSTRING_INDEX(A.`current_revision_name`, '-', 4)
	    WHERE `model`.`delete_time` IS NULL AND `model`.`legacy_model_id` IN ("tm20230314-221943-XBWSTWNA-FMVDNT")
*/
func constructDeployedModelsByTargetTypeQuery(listOptionsExt *apipb.ListOptionsExt) (string, []interface{}, error) {
	filters := getFilterMap(listOptionsExt)
	var deploymentState, targetType, modelList, query string
	var queryParam []interface{}
	deploymentState, ok := filters["deployment.status.state"]
	if !ok {
		return query, queryParam, errors.New("Field: deployment.status.state not provided")
	}
	targetType, ok = filters["deployment.spec.definition.type"]
	if !ok {
		return query, queryParam, errors.New("Field: deployment.spec.definition.type not provided")
	}
	modelList, ok = filters["model.spec.legacy_model_spec.tm_model_id"]
	if !ok {
		return query, queryParam, errors.New("Field: model.spec.legacy_model_spec.tm_model_id not provided")
	}
	query = "WITH A AS (SELECT `current_revision_name` FROM `deployment` WHERE `delete_time` IS NULL AND ("
	valueStr, valueParam, err := convertCriterionOperator("state", apipb.CRITERION_OPERATOR_IN, deploymentState)
	valueStr = valueStr + " " + logicalOperatorMap[apipb.LOGICAL_OPERATOR_AND.String()]
	if err != nil {
		return query, queryParam, status.Errorf(codes.Internal, "error converting deploymentState value query string: %v", err)
	}
	query += valueStr
	queryParam = append(queryParam, valueParam...)

	valueStr, valueParam, err = convertCriterionOperator("target_definition_type", apipb.CRITERION_OPERATOR_IN, targetType)
	valueStr = valueStr + " " + logicalOperatorMap[apipb.LOGICAL_OPERATOR_AND.String()]
	if err != nil {
		return query, queryParam, status.Errorf(codes.Internal, "error converting target_definition_type value query string: %v", err)
	}
	query += valueStr
	query = strings.TrimSuffix(query, "AND")
	queryParam = append(queryParam, valueParam...)
	query += ")) "
	query += "SELECT `name`, `uid`, `group_ver`, `namespace`, `res_version`, `create_time`, `update_time`, `proto` FROM `model` "
	query += "INNER JOIN A ON `model`.`name` = SUBSTRING_INDEX(A.`current_revision_name`, '-', 4) "
	query += "WHERE `model`.`delete_time` IS NULL AND ("

	valueStr, valueParam, err = convertCriterionOperator("legacy_model_id", apipb.CRITERION_OPERATOR_IN, modelList)
	valueStr = valueStr + " " + logicalOperatorMap[apipb.LOGICAL_OPERATOR_AND.String()]
	if err != nil {
		return query, queryParam, status.Errorf(codes.Internal, "error converting legacy_model_id value query string: %v", err)
	}
	query += valueStr
	query = strings.TrimSuffix(query, "AND")
	query += ") "
	queryParam = append(queryParam, valueParam...)
	return query, queryParam, nil
}

// constructModelCardRevisionsForGivenDateQuery constructs the query to get the latest model card revision for each model card for a given date
/*
	Sample Generated Query -
SELECT
  `revision`.`name`,
  `revision`.`uid`,
  `revision`.`group_ver`,
  `revision`.`namespace`,
  `revision`.`res_version`,
  `revision`.`create_time`,
  `revision`.`update_time`,
  `revision`.`proto`
FROM
  `revision`
  INNER JOIN (
    SELECT
      `base_resource_name`,
      MAX(`create_time`) AS max_create_time
    FROM
      `revision`
    WHERE
      `namespace` = '{{namespace}}'
      AND `base_type` = 'ModelCard'
      AND `create_time` <= '{{create_time}}'
    GROUP BY
      `base_resource_name`
  ) AS latest ON `revision`.`base_resource_name` = `latest`.`base_resource_name`
  AND `revision`.`create_time` = `latest`.`max_create_time`
WHERE
  `revision`.`namespace` = '{{namespace}}'
  AND `revision`.`base_type` = 'ModelCard'
*/
func constructModelCardRevisionsForGivenDateQuery(listOptionsExt *apipb.ListOptionsExt) (string, []interface{}, error) {
	criteria := listOptionsExt.GetOperation().GetCriterion()
	createTimeConditions := []string{" AND"}
	var createTimeQueryParams []interface{}
	queryParam := make([]interface{}, 0, 3)
	namespace := "ma-responsible-ai"

	for _, criterion := range criteria {
		if criterion.GetFieldName() == "revision.metadata.creation_timestamp" {
			condition, param, err := convertCriterionOperator("create_time", criterion.GetOperator(), string(criterion.GetMatchValue().GetValue()))
			condition = condition + " " + logicalOperatorMap[apipb.LOGICAL_OPERATOR_AND.String()]
			createTimeConditions = append(createTimeConditions, condition)
			createTimeQueryParams = append(createTimeQueryParams, param...)
			if err != nil {
				return "", nil, err
			}
		}
	}

	// If there is no create_time condition, add the current time as the create_time condition
	if len(createTimeQueryParams) == 0 {
		timestamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")
		createTimeConditions = append(createTimeConditions, " `create_time` <= ? ")
		createTimeQueryParams = append(createTimeQueryParams, timestamp)
	}
	// Select Statement
	query := "SELECT `revision`.`name`, `revision`.`uid`, `revision`.`group_ver`, `revision`.`namespace`, `revision`.`res_version`, `revision`.`create_time`, `revision`.`update_time`, `revision`.`proto` FROM `revision` "

	// Inner Join Statement
	query += "INNER JOIN ( "

	// Inner Join Base Query
	query += "SELECT `base_resource_name`, MAX(`create_time`) AS max_create_time FROM `revision` WHERE `namespace` = ? AND `base_type` = 'ModelCard'"
	queryParam = append(queryParam, namespace)

	// Inner Join create_time conditions
	query += strings.Join(createTimeConditions, "")
	queryParam = append(queryParam, createTimeQueryParams...)
	query = strings.TrimSuffix(query, "AND")

	// Inner Join Group By
	query += "GROUP BY `base_resource_name` "

	// Inner Join Condition
	query += ") AS latest ON `revision`.`base_resource_name` = `latest`.`base_resource_name` AND `revision`.`create_time` = `latest`.`max_create_time` "
	// Outer Query where clause
	query += "WHERE `revision`.`namespace` = ? AND `revision`.`base_type` = 'ModelCard' "
	queryParam = append(queryParam, namespace)

	return query, queryParam, nil
}
