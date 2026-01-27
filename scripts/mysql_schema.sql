-- MySQL Schema for Michelangelo Ingester (Sandbox)
-- This is a simplified schema focusing on the most commonly used CRDs

-- Drop tables if they exist (for clean reinstall)
SET FOREIGN_KEY_CHECKS = 0;

DROP TABLE IF EXISTS `model_annotations`;
DROP TABLE IF EXISTS `model_labels`;
DROP TABLE IF EXISTS `model`;

DROP TABLE IF EXISTS `pipeline_annotations`;
DROP TABLE IF EXISTS `pipeline_labels`;
DROP TABLE IF EXISTS `pipeline`;

DROP TABLE IF EXISTS `pipeline_run_annotations`;
DROP TABLE IF EXISTS `pipeline_run_labels`;
DROP TABLE IF EXISTS `pipeline_run`;

DROP TABLE IF EXISTS `dataset_annotations`;
DROP TABLE IF EXISTS `dataset_labels`;
DROP TABLE IF EXISTS `dataset`;

DROP TABLE IF EXISTS `deployment_annotations`;
DROP TABLE IF EXISTS `deployment_labels`;
DROP TABLE IF EXISTS `deployment`;

SET FOREIGN_KEY_CHECKS = 1;

-- ==============================================================================
-- MODEL TABLE
-- ==============================================================================
CREATE TABLE `model`
(
    `uid`         VARCHAR(255) NOT NULL COMMENT 'Kubernetes UID',
    `group_ver`   VARCHAR(255) NOT NULL COMMENT 'API group version',
    `namespace`   VARCHAR(255) NOT NULL COMMENT 'Kubernetes namespace',
    `name`        VARCHAR(255) NOT NULL COMMENT 'Object name',
    `res_version` BIGINT UNSIGNED NOT NULL COMMENT 'Resource version',
    `create_time` DATETIME     NOT NULL COMMENT 'Creation timestamp',
    `update_time` DATETIME     COMMENT 'Last update timestamp',
    `delete_time` DATETIME     COMMENT 'Deletion timestamp (soft delete)',
    `proto`       MEDIUMBLOB   COMMENT 'Protobuf serialized object',
    `json`        JSON         COMMENT 'JSON representation',

    -- Model-specific indexed fields
    `algorithm`    VARCHAR(255) COMMENT 'Training algorithm',
    `training_framework`    VARCHAR(255) COMMENT 'ML framework (e.g., TensorFlow, PyTorch)',
    `owner`    VARCHAR(255) COMMENT 'Model owner email',
    `source`    VARCHAR(255) COMMENT 'Model source',
    `description`    VARCHAR(768) COMMENT 'Model description',

    PRIMARY KEY   (`uid`),
    KEY    `model_namespace_name` (`namespace`, `name`),
    KEY    `model_create_time` (`create_time`),
    KEY    `model_update_time` (`update_time`),
    KEY    `model_delete_time` (`delete_time`),
    KEY    `model_namespace_timestamp` (`namespace`, `delete_time`, `create_time`, `update_time`),
    KEY    `model_algorithm` (`algorithm`),
    KEY    `model_training_framework` (`training_framework`),
    KEY    `model_owner` (`owner`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `model_labels`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY    `model_labels_uid` (`obj_uid`),
    KEY    `model_labels_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `model_annotations`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   TEXT,
    PRIMARY KEY (`id`),
    KEY    `model_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- PIPELINE TABLE
-- ==============================================================================
CREATE TABLE `pipeline`
(
    `uid`         VARCHAR(255) NOT NULL,
    `group_ver`   VARCHAR(255) NOT NULL,
    `namespace`   VARCHAR(255) NOT NULL,
    `name`        VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME     NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto`       MEDIUMBLOB,
    `json`        JSON,

    -- Pipeline-specific indexed fields
    `pipeline_type`    VARCHAR(255) COMMENT 'Pipeline type (train, batch_predict, etc.)',
    `owner`    VARCHAR(255) COMMENT 'Pipeline owner',

    PRIMARY KEY   (`uid`),
    KEY    `pipeline_namespace_name` (`namespace`, `name`),
    KEY    `pipeline_create_time` (`create_time`),
    KEY    `pipeline_update_time` (`update_time`),
    KEY    `pipeline_delete_time` (`delete_time`),
    KEY    `pipeline_namespace_timestamp` (`namespace`, `delete_time`, `create_time`, `update_time`),
    KEY    `pipeline_pipeline_type` (`pipeline_type`),
    KEY    `pipeline_owner` (`owner`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `pipeline_labels`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY    `pipeline_labels_uid` (`obj_uid`),
    KEY    `pipeline_labels_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `pipeline_annotations`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   TEXT,
    PRIMARY KEY (`id`),
    KEY    `pipeline_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- PIPELINE_RUN TABLE
-- ==============================================================================
CREATE TABLE `pipeline_run`
(
    `uid`         VARCHAR(255) NOT NULL,
    `group_ver`   VARCHAR(255) NOT NULL,
    `namespace`   VARCHAR(255) NOT NULL,
    `name`        VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME     NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto`       MEDIUMBLOB,
    `json`        JSON,

    -- PipelineRun-specific indexed fields
    `pipeline_namespace`    VARCHAR(255) COMMENT 'Parent pipeline namespace',
    `pipeline_name`    VARCHAR(255) COMMENT 'Parent pipeline name',
    `status`    VARCHAR(255) COMMENT 'Run status (Running, Succeeded, Failed)',
    `owner`    VARCHAR(255) COMMENT 'Run owner',

    PRIMARY KEY   (`uid`),
    KEY    `pipeline_run_namespace_name` (`namespace`, `name`),
    KEY    `pipeline_run_create_time` (`create_time`),
    KEY    `pipeline_run_update_time` (`update_time`),
    KEY    `pipeline_run_delete_time` (`delete_time`),
    KEY    `pipeline_run_namespace_timestamp` (`namespace`, `delete_time`, `create_time`, `update_time`),
    KEY    `pipeline_run_pipeline` (`pipeline_namespace`, `pipeline_name`),
    KEY    `pipeline_run_status` (`status`),
    KEY    `pipeline_run_owner` (`owner`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `pipeline_run_labels`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY    `pipeline_run_labels_uid` (`obj_uid`),
    KEY    `pipeline_run_labels_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `pipeline_run_annotations`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   TEXT,
    PRIMARY KEY (`id`),
    KEY    `pipeline_run_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- DATASET TABLE
-- ==============================================================================
CREATE TABLE `dataset`
(
    `uid`         VARCHAR(255) NOT NULL,
    `group_ver`   VARCHAR(255) NOT NULL,
    `namespace`   VARCHAR(255) NOT NULL,
    `name`        VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME     NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto`       MEDIUMBLOB,
    `json`        JSON,

    -- Dataset-specific indexed fields
    `dataset_type`    VARCHAR(255) COMMENT 'Dataset type',
    `owner`    VARCHAR(255) COMMENT 'Dataset owner',

    PRIMARY KEY   (`uid`),
    KEY    `dataset_namespace_name` (`namespace`, `name`),
    KEY    `dataset_create_time` (`create_time`),
    KEY    `dataset_update_time` (`update_time`),
    KEY    `dataset_delete_time` (`delete_time`),
    KEY    `dataset_namespace_timestamp` (`namespace`, `delete_time`, `create_time`, `update_time`),
    KEY    `dataset_dataset_type` (`dataset_type`),
    KEY    `dataset_owner` (`owner`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `dataset_labels`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY    `dataset_labels_uid` (`obj_uid`),
    KEY    `dataset_labels_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `dataset_annotations`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   TEXT,
    PRIMARY KEY (`id`),
    KEY    `dataset_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- DEPLOYMENT TABLE
-- ==============================================================================
CREATE TABLE `deployment`
(
    `uid`         VARCHAR(255) NOT NULL,
    `group_ver`   VARCHAR(255) NOT NULL,
    `namespace`   VARCHAR(255) NOT NULL,
    `name`        VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME     NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto`       MEDIUMBLOB,
    `json`        JSON,

    -- Deployment-specific indexed fields
    `deployment_type`    VARCHAR(255) COMMENT 'Deployment type',
    `owner`    VARCHAR(255) COMMENT 'Deployment owner',
    `status`    VARCHAR(255) COMMENT 'Deployment status',

    PRIMARY KEY   (`uid`),
    KEY    `deployment_namespace_name` (`namespace`, `name`),
    KEY    `deployment_create_time` (`create_time`),
    KEY    `deployment_update_time` (`update_time`),
    KEY    `deployment_delete_time` (`delete_time`),
    KEY    `deployment_namespace_timestamp` (`namespace`, `delete_time`, `create_time`, `update_time`),
    KEY    `deployment_deployment_type` (`deployment_type`),
    KEY    `deployment_owner` (`owner`),
    KEY    `deployment_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `deployment_labels`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY    `deployment_labels_uid` (`obj_uid`),
    KEY    `deployment_labels_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `deployment_annotations`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   TEXT,
    PRIMARY KEY (`id`),
    KEY    `deployment_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- MODEL_FAMILY TABLE
-- ==============================================================================
CREATE TABLE `model_family`
(
    `uid`         VARCHAR(255) NOT NULL,
    `group_ver`   VARCHAR(255) NOT NULL,
    `namespace`   VARCHAR(255) NOT NULL,
    `name`        VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME     NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto`       MEDIUMBLOB,
    `json`        JSON,
    PRIMARY KEY   (`uid`),
    KEY    `model_family_namespace_name` (`namespace`, `name`),
    KEY    `model_family_create_time` (`create_time`),
    KEY    `model_family_update_time` (`update_time`),
    KEY    `model_family_delete_time` (`delete_time`),
    KEY    `model_family_namespace_timestamp` (`namespace`, `delete_time`, `create_time`, `update_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `model_family_labels`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY    `model_family_labels_uid` (`obj_uid`),
    KEY    `model_family_labels_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `model_family_annotations`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   TEXT,
    PRIMARY KEY (`id`),
    KEY    `model_family_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- INFERENCE_SERVER TABLE
-- ==============================================================================
CREATE TABLE `inference_server`
(
    `uid`         VARCHAR(255) NOT NULL,
    `group_ver`   VARCHAR(255) NOT NULL,
    `namespace`   VARCHAR(255) NOT NULL,
    `name`        VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME     NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto`       MEDIUMBLOB,
    `json`        JSON,
    PRIMARY KEY   (`uid`),
    KEY    `inference_server_namespace_name` (`namespace`, `name`),
    KEY    `inference_server_create_time` (`create_time`),
    KEY    `inference_server_update_time` (`update_time`),
    KEY    `inference_server_delete_time` (`delete_time`),
    KEY    `inference_server_namespace_timestamp` (`namespace`, `delete_time`, `create_time`, `update_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `inference_server_labels`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY    `inference_server_labels_uid` (`obj_uid`),
    KEY    `inference_server_labels_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `inference_server_annotations`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   TEXT,
    PRIMARY KEY (`id`),
    KEY    `inference_server_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- PROJECT TABLE
-- ==============================================================================
CREATE TABLE `project`
(
    `uid`         VARCHAR(255) NOT NULL,
    `group_ver`   VARCHAR(255) NOT NULL,
    `namespace`   VARCHAR(255) NOT NULL,
    `name`        VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME     NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto`       MEDIUMBLOB,
    `json`        JSON,
    PRIMARY KEY   (`uid`),
    KEY    `project_namespace_name` (`namespace`, `name`),
    KEY    `project_create_time` (`create_time`),
    KEY    `project_update_time` (`update_time`),
    KEY    `project_delete_time` (`delete_time`),
    KEY    `project_namespace_timestamp` (`namespace`, `delete_time`, `create_time`, `update_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `project_labels`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY    `project_labels_uid` (`obj_uid`),
    KEY    `project_labels_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `project_annotations`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   TEXT,
    PRIMARY KEY (`id`),
    KEY    `project_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- REVISION TABLE
-- ==============================================================================
CREATE TABLE `revision`
(
    `uid`         VARCHAR(255) NOT NULL,
    `group_ver`   VARCHAR(255) NOT NULL,
    `namespace`   VARCHAR(255) NOT NULL,
    `name`        VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME     NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto`       MEDIUMBLOB,
    `json`        JSON,
    PRIMARY KEY   (`uid`),
    KEY    `revision_namespace_name` (`namespace`, `name`),
    KEY    `revision_create_time` (`create_time`),
    KEY    `revision_update_time` (`update_time`),
    KEY    `revision_delete_time` (`delete_time`),
    KEY    `revision_namespace_timestamp` (`namespace`, `delete_time`, `create_time`, `update_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `revision_labels`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY    `revision_labels_uid` (`obj_uid`),
    KEY    `revision_labels_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `revision_annotations`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   TEXT,
    PRIMARY KEY (`id`),
    KEY    `revision_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- CLUSTER TABLE
-- ==============================================================================
CREATE TABLE `cluster`
(
    `uid`         VARCHAR(255) NOT NULL,
    `group_ver`   VARCHAR(255) NOT NULL,
    `namespace`   VARCHAR(255) NOT NULL,
    `name`        VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME     NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto`       MEDIUMBLOB,
    `json`        JSON,
    PRIMARY KEY   (`uid`),
    KEY    `cluster_namespace_name` (`namespace`, `name`),
    KEY    `cluster_create_time` (`create_time`),
    KEY    `cluster_update_time` (`update_time`),
    KEY    `cluster_delete_time` (`delete_time`),
    KEY    `cluster_namespace_timestamp` (`namespace`, `delete_time`, `create_time`, `update_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `cluster_labels`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY    `cluster_labels_uid` (`obj_uid`),
    KEY    `cluster_labels_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `cluster_annotations`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   TEXT,
    PRIMARY KEY (`id`),
    KEY    `cluster_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- RAY_CLUSTER TABLE
-- ==============================================================================
CREATE TABLE `ray_cluster`
(
    `uid`         VARCHAR(255) NOT NULL,
    `group_ver`   VARCHAR(255) NOT NULL,
    `namespace`   VARCHAR(255) NOT NULL,
    `name`        VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME     NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto`       MEDIUMBLOB,
    `json`        JSON,
    PRIMARY KEY   (`uid`),
    KEY    `ray_cluster_namespace_name` (`namespace`, `name`),
    KEY    `ray_cluster_create_time` (`create_time`),
    KEY    `ray_cluster_update_time` (`update_time`),
    KEY    `ray_cluster_delete_time` (`delete_time`),
    KEY    `ray_cluster_namespace_timestamp` (`namespace`, `delete_time`, `create_time`, `update_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `ray_cluster_labels`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY    `ray_cluster_labels_uid` (`obj_uid`),
    KEY    `ray_cluster_labels_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `ray_cluster_annotations`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   TEXT,
    PRIMARY KEY (`id`),
    KEY    `ray_cluster_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- RAY_JOB TABLE
-- ==============================================================================
CREATE TABLE `ray_job`
(
    `uid`         VARCHAR(255) NOT NULL,
    `group_ver`   VARCHAR(255) NOT NULL,
    `namespace`   VARCHAR(255) NOT NULL,
    `name`        VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME     NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto`       MEDIUMBLOB,
    `json`        JSON,
    PRIMARY KEY   (`uid`),
    KEY    `ray_job_namespace_name` (`namespace`, `name`),
    KEY    `ray_job_create_time` (`create_time`),
    KEY    `ray_job_update_time` (`update_time`),
    KEY    `ray_job_delete_time` (`delete_time`),
    KEY    `ray_job_namespace_timestamp` (`namespace`, `delete_time`, `create_time`, `update_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `ray_job_labels`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY    `ray_job_labels_uid` (`obj_uid`),
    KEY    `ray_job_labels_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `ray_job_annotations`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   TEXT,
    PRIMARY KEY (`id`),
    KEY    `ray_job_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- SPARK_JOB TABLE
-- ==============================================================================
CREATE TABLE `spark_job`
(
    `uid`         VARCHAR(255) NOT NULL,
    `group_ver`   VARCHAR(255) NOT NULL,
    `namespace`   VARCHAR(255) NOT NULL,
    `name`        VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME     NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto`       MEDIUMBLOB,
    `json`        JSON,
    PRIMARY KEY   (`uid`),
    KEY    `spark_job_namespace_name` (`namespace`, `name`),
    KEY    `spark_job_create_time` (`create_time`),
    KEY    `spark_job_update_time` (`update_time`),
    KEY    `spark_job_delete_time` (`delete_time`),
    KEY    `spark_job_namespace_timestamp` (`namespace`, `delete_time`, `create_time`, `update_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `spark_job_labels`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY    `spark_job_labels_uid` (`obj_uid`),
    KEY    `spark_job_labels_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `spark_job_annotations`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   TEXT,
    PRIMARY KEY (`id`),
    KEY    `spark_job_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==============================================================================
-- TRIGGER_RUN TABLE
-- ==============================================================================
CREATE TABLE `trigger_run`
(
    `uid`         VARCHAR(255) NOT NULL,
    `group_ver`   VARCHAR(255) NOT NULL,
    `namespace`   VARCHAR(255) NOT NULL,
    `name`        VARCHAR(255) NOT NULL,
    `res_version` BIGINT UNSIGNED NOT NULL,
    `create_time` DATETIME     NOT NULL,
    `update_time` DATETIME,
    `delete_time` DATETIME,
    `proto`       MEDIUMBLOB,
    `json`        JSON,
    PRIMARY KEY   (`uid`),
    KEY    `trigger_run_namespace_name` (`namespace`, `name`),
    KEY    `trigger_run_create_time` (`create_time`),
    KEY    `trigger_run_update_time` (`update_time`),
    KEY    `trigger_run_delete_time` (`delete_time`),
    KEY    `trigger_run_namespace_timestamp` (`namespace`, `delete_time`, `create_time`, `update_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `trigger_run_labels`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY    `trigger_run_labels_uid` (`obj_uid`),
    KEY    `trigger_run_labels_value` (`key`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `trigger_run_annotations`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   TEXT,
    PRIMARY KEY (`id`),
    KEY    `trigger_run_annotations_uid` (`obj_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
