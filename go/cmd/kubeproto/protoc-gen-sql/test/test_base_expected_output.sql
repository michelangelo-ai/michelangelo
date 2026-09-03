CREATE TABLE `test_base`
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
    `test_name`    VARCHAR(255),
    `test_ref_namespace`    VARCHAR(255),
    `test_ref_name`    VARCHAR(255),
    `test_count`    INT,
    PRIMARY KEY   (`uid`),
    KEY    `test_base_namespace_name` (`namespace`, `name`),
    KEY    `test_base_create_time` (`create_time`),
    KEY    `test_base_update_time` (`update_time`),
    KEY    `test_base_delete_time` (`delete_time`),
    KEY    `test_base_namespace_timestamp` (`namespace`, `delete_time`, `create_time`, `update_time`),
    KEY    `test_base_test_name` (`test_name`),
    KEY    `test_base_test_ref` (`test_ref_namespace`, `test_ref_name`),
    KEY    `test_base_test_count` (`test_count`)
);
CREATE TABLE `test_base_labels`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY    `test_base_labels_uid` (`obj_uid`),
    KEY    `test_base_labels_value` (`key`, `value`)
);
CREATE TABLE `test_base_annotations`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   TEXT,
    PRIMARY KEY (`id`),
    KEY    `test_base_annotations_uid` (`obj_uid`)
);
CREATE TABLE `test_base_test_wrapper_unmarshalled`
(
    `test_wrapper_uid` VARCHAR(255) NOT NULL,
    `name`    VARCHAR(255),
    `test_name`    VARCHAR(255),
    `test_ref_namespace`    VARCHAR(255),
    `test_ref_name`    VARCHAR(255),
    `test_count`    INT,
    PRIMARY KEY (`test_wrapper_uid`),
    KEY    `test_base_test_wrapper_unmarshalled_name` (`name`),
    KEY    `test_base_test_wrapper_unmarshalled_test_name` (`test_name`),
    KEY    `test_base_test_wrapper_unmarshalled_test_ref` (`test_ref_namespace`, `test_ref_name`),
    KEY    `test_base_test_wrapper_unmarshalled_test_count` (`test_count`)
);
