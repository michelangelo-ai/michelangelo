"""Tests for S3A session setup in michelangelo.uniflow.plugins.spark (no JVM needed)."""

from __future__ import annotations

import os
from unittest import TestCase, mock

from michelangelo.uniflow.plugins.spark import io as spark_io
from michelangelo.uniflow.plugins.spark import task as spark_task

_AWS_ENV = {
    "AWS_ACCESS_KEY_ID": "minio",
    "AWS_SECRET_ACCESS_KEY": "minio123",
    "AWS_ENDPOINT_URL": "http://minio:9000",
}


class _FakeBuilder:
    """Records builder calls in order instead of starting a SparkContext."""

    def __init__(self):
        self.configs: list[tuple[str, str]] = []
        self.created = False

    def appName(self, _name):  # noqa: N802 - mirrors the PySpark builder API
        return self

    def enableHiveSupport(self):  # noqa: N802
        return self

    def config(self, key, value):
        self.configs.append((key, value))
        return self

    def getOrCreate(self):  # noqa: N802
        self.created = True
        return mock.MagicMock()


class TestS3ASparkConfig(TestCase):
    """s3a_spark_config() builds the builder-time S3A properties from the env."""

    def test_includes_credentials_and_endpoint_from_env(self):
        """AWS_* env vars become spark.hadoop.fs.s3a.* properties."""
        with mock.patch.dict(os.environ, _AWS_ENV):
            conf = spark_io.s3a_spark_config()
        self.assertEqual(conf["spark.hadoop.fs.s3a.access.key"], "minio")
        self.assertEqual(conf["spark.hadoop.fs.s3a.secret.key"], "minio123")
        self.assertEqual(conf["spark.hadoop.fs.s3a.endpoint"], "http://minio:9000")
        self.assertEqual(conf["spark.hadoop.fs.s3a.path.style.access"], "true")
        self.assertEqual(
            conf["spark.hadoop.fs.s3.impl"], "org.apache.hadoop.fs.s3a.S3AFileSystem"
        )

    def test_omits_unset_credentials(self):
        """Unset AWS_* vars are left out so the default S3A credential chain applies."""
        with mock.patch.dict(os.environ, {}, clear=True):
            conf = spark_io.s3a_spark_config()
        self.assertNotIn("spark.hadoop.fs.s3a.access.key", conf)
        self.assertNotIn("spark.hadoop.fs.s3a.secret.key", conf)
        self.assertNotIn("spark.hadoop.fs.s3a.endpoint", conf)
        self.assertIn("spark.hadoop.fs.s3a.impl", conf)


class TestEnsureS3AConfig(TestCase):
    """_ensure_s3a_config() never touches the private Py4J bridge."""

    def setUp(self):
        """Reset the module-level idempotency flag around each test."""
        spark_io._s3a_configured = False
        self.addCleanup(setattr, spark_io, "_s3a_configured", False)

    def test_creates_session_with_s3a_config_when_none_active(self):
        """With no active session, one is created with the S3A properties."""
        builder = _FakeBuilder()
        with (
            mock.patch.object(spark_io, "SparkSession") as session_cls,
            mock.patch.dict(os.environ, _AWS_ENV),
        ):
            session_cls.getActiveSession.return_value = None
            session_cls.builder = builder
            spark_io._ensure_s3a_config()
        self.assertTrue(builder.created)
        self.assertIn(("spark.hadoop.fs.s3a.access.key", "minio"), builder.configs)

    def test_active_session_is_left_alone(self):
        """An active session is not reconfigured through sparkContext._jsc."""
        active = mock.MagicMock()
        active.conf.get.return_value = "org.apache.hadoop.fs.s3a.S3AFileSystem"
        with mock.patch.object(spark_io, "SparkSession") as session_cls:
            session_cls.getActiveSession.return_value = active
            with self.assertNoLogs(spark_io.log, level="WARNING"):
                spark_io._ensure_s3a_config()
        active.sparkContext._jsc.hadoopConfiguration.assert_not_called()

    def test_warns_when_active_session_lacks_s3a_config(self):
        """A session started without the S3A properties is reported, not patched."""
        active = mock.MagicMock()
        active.conf.get.return_value = None
        with mock.patch.object(spark_io, "SparkSession") as session_cls:
            session_cls.getActiveSession.return_value = active
            with self.assertLogs(spark_io.log, level="WARNING"):
                spark_io._ensure_s3a_config()
        active.sparkContext._jsc.hadoopConfiguration.assert_not_called()

    def test_is_idempotent(self):
        """Only the first call looks up the session."""
        with mock.patch.object(spark_io, "SparkSession") as session_cls:
            session_cls.getActiveSession.return_value = mock.MagicMock()
            spark_io._ensure_s3a_config()
            spark_io._ensure_s3a_config()
        session_cls.getActiveSession.assert_called_once()


class TestSparkTaskPreRun(TestCase):
    """SparkTask.pre_run() applies the S3A properties before getOrCreate()."""

    def test_applies_s3a_config_and_lets_spark_properties_override(self):
        """S3A defaults are set first, so _SPARK_PROPERTIES entries win."""
        builder = _FakeBuilder()
        env = dict(
            _AWS_ENV, _SPARK_PROPERTIES="spark.hadoop.fs.s3a.endpoint=http://other:9000"
        )
        with (
            mock.patch.object(spark_task, "SparkSession") as session_cls,
            mock.patch.dict(os.environ, env),
        ):
            session_cls.builder = builder
            spark_task.SparkTask().pre_run()
        self.assertTrue(builder.created)
        self.assertIn(("spark.hadoop.fs.s3a.access.key", "minio"), builder.configs)
        endpoints = [
            v for k, v in builder.configs if k == "spark.hadoop.fs.s3a.endpoint"
        ]
        self.assertEqual(endpoints, ["http://minio:9000", "http://other:9000"])


class TestModuleSurface(TestCase):
    """The io module exports only its public API."""

    def test_all(self):
        """__all__ lists the public names."""
        self.assertEqual(spark_io.__all__, ["SparkIO", "s3a_spark_config"])

    def test_no_module_level_read_data(self):
        """The orphaned module-level read_data is gone; SparkIO.read_data remains."""
        self.assertFalse(hasattr(spark_io, "read_data"))
        self.assertTrue(callable(spark_io.SparkIO.read_data))
