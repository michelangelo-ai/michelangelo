CREATE TABLE `test_wrapper`
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
    KEY    `test_wrapper_namespace_name` (`namespace`, `name`),
    KEY    `test_wrapper_create_time` (`create_time`),
    KEY    `test_wrapper_update_time` (`update_time`),
    KEY    `test_wrapper_delete_time` (`delete_time`),
    KEY    `test_wrapper_namespace_timestamp` (`namespace`, `delete_time`, `create_time`, `update_time`)
);
CREATE TABLE `test_wrapper_labels`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   VARCHAR(63),
    PRIMARY KEY (`id`),
    KEY    `test_wrapper_labels_uid` (`obj_uid`),
    KEY    `test_wrapper_labels_value` (`key`, `value`)
);
CREATE TABLE `test_wrapper_annotations`
(
    `id`      BIGINT       NOT NULL AUTO_INCREMENT,
    `obj_uid` VARCHAR(255) NOT NULL,
    `key`     VARCHAR(255) NOT NULL,
    `value`   TEXT,
    PRIMARY KEY (`id`),
    KEY    `test_wrapper_annotations_uid` (`obj_uid`)
);
