-- ==============================================================================
-- Michelangelo Ingester - Complete Database Schema
-- ==============================================================================
-- This schema includes ALL 15 CRDs watched by the ingester.
-- Table names are snake_case matching utils.ToSnakeCase(Kind).
-- Safe for production and sandbox (idempotent with IF NOT EXISTS).
-- Generated based on protobuf GetIndexedKeyValuePairs() methods.
-- ==============================================================================

CREATE DATABASE IF NOT EXISTS michelangelo;
USE michelangelo;

-- ==============================================================================
-- 1. MODEL
-- ==============================================================================
CREATE TABLE IF NOT EXISTS `model` (
    `uid` VARCHAR(255) NOT NULL,
    `group_ver` VARCHAR(255) NOT NULL,
    `namespace` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto` MEDIUMBLOB,
    `json` JSON,
    `algorithm` VARCHAR(255),
    `training_framework` VARCHAR(255),
    `owner` VARCHAR(255),
    `source` VARCHAR(255),
    `description` VARCHAR(768),
    `model_kind` VARCHAR(255),
    `package_type` VARCHAR(255),
    `revision_id` VARCHAR(255),
    `src_pipeline_run_namespace` VARCHAR(255),
    `src_pipeline_run_name` VARCHAR(255),
    `model_family_namespace` VARCHAR(255),
    `model_family_name` VARCHAR(255),
    `feature_eval_report_namespace` VARCHAR(255),
    `feature_eval_report_name` VARCHAR(255),
    `performance_eval_report_namespace` VARCHAR(255),
    `performance_eval_report_name` VARCHAR(255),
    `feature_quality_report_namespace` VARCHAR(255),
    `feature_quality_report_name` VARCHAR(255),
    `explainability_report_namespace` VARCHAR(255),
    `explainability_report_name` VARCHAR(255),
    PRIMARY KEY (`uid`),
    KEY `model_namespace_name` (`namespace`, `name`),
    KEY `model_create_time` (`create_time`),
    KEY `model_algorithm` (`algorithm`),
    KEY `model_owner` (`owner`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `model_labels` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY `model_labels_uid` (`obj_uid`),
    KEY `model_labels_key_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `model_annotations` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` TEXT,
    PRIMARY KEY (`id`),
    KEY `model_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- 2. MODEL_FAMILY
-- ==============================================================================
CREATE TABLE IF NOT EXISTS `model_family` (
    `uid` VARCHAR(255) NOT NULL,
    `group_ver` VARCHAR(255) NOT NULL,
    `namespace` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto` MEDIUMBLOB,
    `json` JSON,
    `model_family_name` VARCHAR(255),
    PRIMARY KEY (`uid`),
    KEY `model_family_namespace_name` (`namespace`, `name`),
    KEY `model_family_create_time` (`create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `model_family_labels` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY `model_family_labels_uid` (`obj_uid`),
    KEY `model_family_labels_key_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `model_family_annotations` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` TEXT,
    PRIMARY KEY (`id`),
    KEY `model_family_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- 3. PIPELINE
-- ==============================================================================
CREATE TABLE IF NOT EXISTS `pipeline` (
    `uid` VARCHAR(255) NOT NULL,
    `group_ver` VARCHAR(255) NOT NULL,
    `namespace` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto` MEDIUMBLOB,
    `json` JSON,
    `owner` VARCHAR(255),
    `pipeline_type` VARCHAR(255),
    PRIMARY KEY (`uid`),
    KEY `pipeline_namespace_name` (`namespace`, `name`),
    KEY `pipeline_create_time` (`create_time`),
    KEY `pipeline_owner` (`owner`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `pipeline_labels` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY `pipeline_labels_uid` (`obj_uid`),
    KEY `pipeline_labels_key_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `pipeline_annotations` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` TEXT,
    PRIMARY KEY (`id`),
    KEY `pipeline_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- 4. PIPELINE_RUN
-- ==============================================================================
CREATE TABLE IF NOT EXISTS `pipeline_run` (
    `uid` VARCHAR(255) NOT NULL,
    `group_ver` VARCHAR(255) NOT NULL,
    `namespace` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto` MEDIUMBLOB,
    `json` JSON,
    `pipeline_namespace` VARCHAR(255),
    `pipeline_name` VARCHAR(255),
    `revision_namespace` VARCHAR(255),
    `revision_name` VARCHAR(255),
    `resume_pipeline_run_namespace` VARCHAR(255),
    `resume_pipeline_run_name` VARCHAR(255),
    `state` VARCHAR(255),
    `actor` VARCHAR(255),
    `end_time` DATETIME,
    `exception_type` VARCHAR(255),
    PRIMARY KEY (`uid`),
    KEY `pipeline_run_namespace_name` (`namespace`, `name`),
    KEY `pipeline_run_create_time` (`create_time`),
    KEY `pipeline_run_pipeline` (`pipeline_namespace`, `pipeline_name`),
    KEY `pipeline_run_state` (`state`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `pipeline_run_labels` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY `pipeline_run_labels_uid` (`obj_uid`),
    KEY `pipeline_run_labels_key_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `pipeline_run_annotations` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` TEXT,
    PRIMARY KEY (`id`),
    KEY `pipeline_run_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- 5. DEPLOYMENT
-- ==============================================================================
CREATE TABLE IF NOT EXISTS `deployment` (
    `uid` VARCHAR(255) NOT NULL,
    `group_ver` VARCHAR(255) NOT NULL,
    `namespace` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto` MEDIUMBLOB,
    `json` JSON,
    `state` VARCHAR(255),
    `target_definition_type` VARCHAR(255),
    `current_revision_namespace` VARCHAR(255),
    `current_revision_name` VARCHAR(255),
    `deletion_requested_timestamp` DATETIME,
    PRIMARY KEY (`uid`),
    KEY `deployment_namespace_name` (`namespace`, `name`),
    KEY `deployment_create_time` (`create_time`),
    KEY `deployment_state` (`state`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `deployment_labels` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY `deployment_labels_uid` (`obj_uid`),
    KEY `deployment_labels_key_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `deployment_annotations` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` TEXT,
    PRIMARY KEY (`id`),
    KEY `deployment_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- 6. INFERENCE_SERVER
-- ==============================================================================
CREATE TABLE IF NOT EXISTS `inference_server` (
    `uid` VARCHAR(255) NOT NULL,
    `group_ver` VARCHAR(255) NOT NULL,
    `namespace` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto` MEDIUMBLOB,
    `json` JSON,
    `state` VARCHAR(255),
    PRIMARY KEY (`uid`),
    KEY `inference_server_namespace_name` (`namespace`, `name`),
    KEY `inference_server_create_time` (`create_time`),
    KEY `inference_server_state` (`state`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `inference_server_labels` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY `inference_server_labels_uid` (`obj_uid`),
    KEY `inference_server_labels_key_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `inference_server_annotations` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` TEXT,
    PRIMARY KEY (`id`),
    KEY `inference_server_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- 7. PROJECT
-- ==============================================================================
CREATE TABLE IF NOT EXISTS `project` (
    `uid` VARCHAR(255) NOT NULL,
    `group_ver` VARCHAR(255) NOT NULL,
    `namespace` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto` MEDIUMBLOB,
    `json` JSON,
    `tier` VARCHAR(255),
    PRIMARY KEY (`uid`),
    KEY `project_namespace_name` (`namespace`, `name`),
    KEY `project_create_time` (`create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `project_labels` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY `project_labels_uid` (`obj_uid`),
    KEY `project_labels_key_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `project_annotations` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` TEXT,
    PRIMARY KEY (`id`),
    KEY `project_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- 8. REVISION
-- ==============================================================================
CREATE TABLE IF NOT EXISTS `revision` (
    `uid` VARCHAR(255) NOT NULL,
    `group_ver` VARCHAR(255) NOT NULL,
    `namespace` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto` MEDIUMBLOB,
    `json` JSON,
    `base_resource_namespace` VARCHAR(255),
    `base_resource_name` VARCHAR(255),
    `base_type` VARCHAR(255),
    `commit_branch` VARCHAR(255),
    `git_ref` VARCHAR(255),
    `owner` VARCHAR(255),
    PRIMARY KEY (`uid`),
    KEY `revision_namespace_name` (`namespace`, `name`),
    KEY `revision_create_time` (`create_time`),
    KEY `revision_base_resource` (`base_resource_namespace`, `base_resource_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `revision_labels` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY `revision_labels_uid` (`obj_uid`),
    KEY `revision_labels_key_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `revision_annotations` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` TEXT,
    PRIMARY KEY (`id`),
    KEY `revision_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- 9. CLUSTER
-- ==============================================================================
CREATE TABLE IF NOT EXISTS `cluster` (
    `uid` VARCHAR(255) NOT NULL,
    `group_ver` VARCHAR(255) NOT NULL,
    `namespace` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto` MEDIUMBLOB,
    `json` JSON,
    PRIMARY KEY (`uid`),
    KEY `cluster_namespace_name` (`namespace`, `name`),
    KEY `cluster_create_time` (`create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `cluster_labels` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY `cluster_labels_uid` (`obj_uid`),
    KEY `cluster_labels_key_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `cluster_annotations` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` TEXT,
    PRIMARY KEY (`id`),
    KEY `cluster_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- 10. RAY_CLUSTER
-- ==============================================================================
CREATE TABLE IF NOT EXISTS `ray_cluster` (
    `uid` VARCHAR(255) NOT NULL,
    `group_ver` VARCHAR(255) NOT NULL,
    `namespace` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto` MEDIUMBLOB,
    `json` JSON,
    PRIMARY KEY (`uid`),
    KEY `ray_cluster_namespace_name` (`namespace`, `name`),
    KEY `ray_cluster_create_time` (`create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `ray_cluster_labels` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY `ray_cluster_labels_uid` (`obj_uid`),
    KEY `ray_cluster_labels_key_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `ray_cluster_annotations` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` TEXT,
    PRIMARY KEY (`id`),
    KEY `ray_cluster_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- 11. RAY_JOB
-- ==============================================================================
CREATE TABLE IF NOT EXISTS `ray_job` (
    `uid` VARCHAR(255) NOT NULL,
    `group_ver` VARCHAR(255) NOT NULL,
    `namespace` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto` MEDIUMBLOB,
    `json` JSON,
    PRIMARY KEY (`uid`),
    KEY `ray_job_namespace_name` (`namespace`, `name`),
    KEY `ray_job_create_time` (`create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `ray_job_labels` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY `ray_job_labels_uid` (`obj_uid`),
    KEY `ray_job_labels_key_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `ray_job_annotations` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` TEXT,
    PRIMARY KEY (`id`),
    KEY `ray_job_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- 12. SPARK_JOB
-- ==============================================================================
CREATE TABLE IF NOT EXISTS `spark_job` (
    `uid` VARCHAR(255) NOT NULL,
    `group_ver` VARCHAR(255) NOT NULL,
    `namespace` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto` MEDIUMBLOB,
    `json` JSON,
    PRIMARY KEY (`uid`),
    KEY `spark_job_namespace_name` (`namespace`, `name`),
    KEY `spark_job_create_time` (`create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `spark_job_labels` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY `spark_job_labels_uid` (`obj_uid`),
    KEY `spark_job_labels_key_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `spark_job_annotations` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` TEXT,
    PRIMARY KEY (`id`),
    KEY `spark_job_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- 13. TRIGGER_RUN
-- ==============================================================================
CREATE TABLE IF NOT EXISTS `trigger_run` (
    `uid` VARCHAR(255) NOT NULL,
    `group_ver` VARCHAR(255) NOT NULL,
    `namespace` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto` MEDIUMBLOB,
    `json` JSON,
    `pipeline_namespace` VARCHAR(255),
    `pipeline_name` VARCHAR(255),
    `revision_namespace` VARCHAR(255),
    `revision_name` VARCHAR(255),
    `state` VARCHAR(255),
    `auto_flip` VARCHAR(255),
    PRIMARY KEY (`uid`),
    KEY `trigger_run_namespace_name` (`namespace`, `name`),
    KEY `trigger_run_create_time` (`create_time`),
    KEY `trigger_run_pipeline` (`pipeline_namespace`, `pipeline_name`),
    KEY `trigger_run_state` (`state`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `trigger_run_labels` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY `trigger_run_labels_uid` (`obj_uid`),
    KEY `trigger_run_labels_key_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `trigger_run_annotations` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` TEXT,
    PRIMARY KEY (`id`),
    KEY `trigger_run_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- 14. CACHED_OUTPUT
-- ==============================================================================
CREATE TABLE IF NOT EXISTS `cached_output` (
    `uid` VARCHAR(255) NOT NULL,
    `group_ver` VARCHAR(255) NOT NULL,
    `namespace` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto` MEDIUMBLOB,
    `json` JSON,
    PRIMARY KEY (`uid`),
    KEY `cached_output_namespace_name` (`namespace`, `name`),
    KEY `cached_output_create_time` (`create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `cached_output_labels` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY `cached_output_labels_uid` (`obj_uid`),
    KEY `cached_output_labels_key_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `cached_output_annotations` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` TEXT,
    PRIMARY KEY (`id`),
    KEY `cached_output_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- 15. EVALUATION_REPORT
-- ==============================================================================
CREATE TABLE IF NOT EXISTS `evaluation_report` (
    `uid` VARCHAR(255) NOT NULL,
    `group_ver` VARCHAR(255) NOT NULL,
    `namespace` VARCHAR(255) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto` MEDIUMBLOB,
    `json` JSON,
    PRIMARY KEY (`uid`),
    KEY `evaluation_report_namespace_name` (`namespace`, `name`),
    KEY `evaluation_report_create_time` (`create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `evaluation_report_labels` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY `evaluation_report_labels_uid` (`obj_uid`),
    KEY `evaluation_report_labels_key_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `evaluation_report_annotations` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key` VARCHAR(255) NOT NULL,
    `value` TEXT,
    PRIMARY KEY (`id`),
    KEY `evaluation_report_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- SUMMARY
-- ==============================================================================
SELECT 'Complete schema initialization finished!' as status;
SELECT COUNT(*) as table_count FROM information_schema.tables
WHERE table_schema = 'michelangelo';

SELECT table_name, table_rows
FROM information_schema.tables
WHERE table_schema = 'michelangelo'
ORDER BY table_name;
