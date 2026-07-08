"""Tests for michelangelo.workflow.schema.pusher config dataclasses."""

from __future__ import annotations

from unittest import TestCase

from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.pusher import (
    DatasetFormat,
    DatasetPluginConfig,
    EvalReportPluginConfig,
    ModelPluginConfig,
    PusherConfig,
    PusherPluginConfig,
)


class TestDatasetFormat(TestCase):
    """Tests for the DatasetFormat enum."""

    def test_values(self):
        """It exposes csv, parquet, and json string values."""
        self.assertEqual(DatasetFormat.CSV.value, "csv")
        self.assertEqual(DatasetFormat.PARQUET.value, "parquet")
        self.assertEqual(DatasetFormat.JSON.value, "json")


class TestModelPluginConfig(TestCase):
    """Tests for ModelPluginConfig defaults and field storage."""

    def test_defaults(self):
        """model_name, description, run_id default to None; labels, metadata to {}."""
        cfg = ModelPluginConfig()
        self.assertIsNone(cfg.model_name)
        self.assertIsNone(cfg.description)
        self.assertIsNone(cfg.run_id)
        self.assertEqual(cfg.labels, {})
        self.assertEqual(cfg.metadata, {})

    def test_labels_instances_are_independent(self):
        """It creates a separate labels dict for each instance."""
        a = ModelPluginConfig()
        b = ModelPluginConfig()
        a.labels["k"] = "v"
        self.assertEqual(b.labels, {})

    def test_stores_provided_values(self):
        """It stores model_name, description, labels (str values only), and run_id."""
        cfg = ModelPluginConfig(
            model_name="clf",
            description="A classifier",
            labels={"framework": "xgboost", "owner": "ml-platform"},
            run_id="run-abc123",
        )
        self.assertEqual(cfg.model_name, "clf")
        self.assertEqual(cfg.description, "A classifier")
        self.assertEqual(cfg.labels["framework"], "xgboost")
        self.assertEqual(cfg.labels["owner"], "ml-platform")
        self.assertEqual(cfg.run_id, "run-abc123")


class TestDatasetPluginConfig(TestCase):
    """Tests for DatasetPluginConfig defaults and field storage."""

    def test_defaults(self):
        """It defaults destination_path to None, format to PARQUET."""
        cfg = DatasetPluginConfig()
        self.assertIsNone(cfg.destination_path)
        self.assertEqual(cfg.format, DatasetFormat.PARQUET)
        self.assertEqual(cfg.partition_by, [])

    def test_partition_by_instances_are_independent(self):
        """It creates a separate partition_by list for each instance."""
        a = DatasetPluginConfig()
        b = DatasetPluginConfig()
        a.partition_by.append("col")
        self.assertEqual(b.partition_by, [])


class TestEvalReportPluginConfig(TestCase):
    """Tests for EvalReportPluginConfig defaults and field storage."""

    def test_defaults(self):
        """It defaults report_name to None and extra_fields to {}."""
        cfg = EvalReportPluginConfig()
        self.assertIsNone(cfg.report_name)
        self.assertEqual(cfg.extra_fields, {})

    def test_extra_fields_instances_are_independent(self):
        """It creates a separate extra_fields dict for each instance."""
        a = EvalReportPluginConfig()
        b = EvalReportPluginConfig()
        a.extra_fields["k"] = "v"
        self.assertEqual(b.extra_fields, {})


class TestPusherPluginConfigResolvedPluginName(TestCase):
    """Tests for PusherPluginConfig.resolved_plugin_name()."""

    def test_returns_model_plugin(self):
        """It returns 'model_plugin' when model_plugin is set."""
        cfg = PusherPluginConfig(name="m", model_plugin=ModelPluginConfig())
        self.assertEqual(cfg.resolved_plugin_name(), "model_plugin")

    def test_returns_dataset_plugin(self):
        """It returns 'dataset_plugin' when dataset_plugin is set."""
        cfg = PusherPluginConfig(name="d", dataset_plugin=DatasetPluginConfig())
        self.assertEqual(cfg.resolved_plugin_name(), "dataset_plugin")

    def test_returns_eval_report_plugin(self):
        """It returns 'eval_report_plugin' when eval_report_plugin is set."""
        cfg = PusherPluginConfig(name="e", eval_report_plugin=EvalReportPluginConfig())
        self.assertEqual(cfg.resolved_plugin_name(), "eval_report_plugin")

    def test_returns_plugin_name_for_extension_plugin(self):
        """It returns plugin_name when a provider-registered plugin is used."""
        cfg = PusherPluginConfig(
            name="x",
            plugin_name="custom_hive_plugin",
            plugin_config={"table": "ml"},
        )
        self.assertEqual(cfg.resolved_plugin_name(), "custom_hive_plugin")

    def test_raises_when_no_plugin_set(self):
        """It raises ConfigurationError when no plugin is specified."""
        cfg = PusherPluginConfig(name="m")
        with self.assertRaises(ConfigurationError) as ctx:
            cfg.resolved_plugin_name()
        self.assertIn("No plugin specified", str(ctx.exception))
        self.assertIn("m", str(ctx.exception))

    def test_raises_when_two_builtin_plugins_set(self):
        """It raises ConfigurationError when two built-in plugin configs are set."""
        cfg = PusherPluginConfig(
            name="m",
            model_plugin=ModelPluginConfig(),
            dataset_plugin=DatasetPluginConfig(),
        )
        with self.assertRaises(ConfigurationError) as ctx:
            cfg.resolved_plugin_name()
        self.assertIn("multiple plugin configs", str(ctx.exception))

    def test_raises_when_builtin_and_plugin_name_both_set(self):
        """It raises ConfigurationError when a typed field and plugin_name are set."""
        cfg = PusherPluginConfig(
            name="m",
            model_plugin=ModelPluginConfig(),
            plugin_name="custom",
            plugin_config={"key": "val"},
        )
        with self.assertRaises(ConfigurationError) as ctx:
            cfg.resolved_plugin_name()
        self.assertIn("plugin_name", str(ctx.exception))


class TestPusherPluginConfigResolvedPluginConfig(TestCase):
    """Tests for PusherPluginConfig.resolved_plugin_config()."""

    def test_returns_typed_config_for_builtin_plugin(self):
        """It returns the typed config dataclass for a built-in plugin."""
        model_cfg = ModelPluginConfig(model_name="clf")
        cfg = PusherPluginConfig(name="m", model_plugin=model_cfg)
        self.assertIs(cfg.resolved_plugin_config(), model_cfg)

    def test_returns_raw_dict_for_extension_plugin(self):
        """It returns plugin_config dict when plugin_name is a non-field name."""
        raw = {"database": "ml_evals", "table": "runs"}
        cfg = PusherPluginConfig(
            name="data",
            plugin_name="genai_evaluation_hive_plugin",
            plugin_config=raw,
        )
        # getattr(self, "genai_evaluation_hive_plugin", None) returns None
        # — must fall through to self.plugin_config, not return None.
        self.assertIs(cfg.resolved_plugin_config(), raw)


class TestPusherPluginConfigBuiltinFieldsClassVar(TestCase):
    """Tests that _BUILTIN_FIELDS is a ClassVar, not an instance field."""

    def test_builtin_fields_not_in_init_signature(self):
        """It is not settable via the constructor (ClassVar excluded by dataclass)."""
        import inspect

        params = inspect.signature(PusherPluginConfig.__init__).parameters
        self.assertNotIn("_BUILTIN_FIELDS", params)

    def test_builtin_fields_shared_across_instances(self):
        """It is shared at class level — the same object on every instance."""
        a = PusherPluginConfig(name="a", model_plugin=ModelPluginConfig())
        b = PusherPluginConfig(name="b", dataset_plugin=DatasetPluginConfig())
        self.assertIs(a._BUILTIN_FIELDS, b._BUILTIN_FIELDS)


class TestPusherConfig(TestCase):
    """Tests for PusherConfig."""

    def test_items_defaults_to_empty_list(self):
        """It defaults items to an empty list."""
        cfg = PusherConfig()
        self.assertEqual(cfg.items, [])

    def test_items_instances_are_independent(self):
        """It creates a separate items list for each instance."""
        a = PusherConfig()
        b = PusherConfig()
        a.items.append(PusherPluginConfig(name="x", model_plugin=ModelPluginConfig()))
        self.assertEqual(b.items, [])

    def test_stores_provided_items(self):
        """It stores a list of PusherPluginConfig items."""
        item = PusherPluginConfig(name="m", model_plugin=ModelPluginConfig())
        cfg = PusherConfig(items=[item])
        self.assertEqual(len(cfg.items), 1)
        self.assertEqual(cfg.items[0].name, "m")

    def test_raises_on_duplicate_artifact_names(self):
        """It raises ConfigurationError when two items share the same name."""
        item_a = PusherPluginConfig(name="model", model_plugin=ModelPluginConfig())
        item_b = PusherPluginConfig(name="model", model_plugin=ModelPluginConfig())
        with self.assertRaises(ConfigurationError) as ctx:
            PusherConfig(items=[item_a, item_b])
        self.assertIn("model", str(ctx.exception))

    def test_accepts_items_with_unique_names(self):
        """It does not raise when all artifact names are unique."""
        a = PusherPluginConfig(name="m", model_plugin=ModelPluginConfig())
        b = PusherPluginConfig(name="d", dataset_plugin=DatasetPluginConfig())
        cfg = PusherConfig(items=[a, b])
        self.assertEqual(len(cfg.items), 2)


class TestPusherPluginConfigPostInit(TestCase):
    """Tests for PusherPluginConfig.__post_init__ paired validation."""

    def test_raises_when_plugin_name_set_without_plugin_config(self):
        """It raises when plugin_name is set but plugin_config is None."""
        with self.assertRaises(ConfigurationError) as ctx:
            PusherPluginConfig(name="x", plugin_name="hive_plugin")
        self.assertIn("plugin_name and plugin_config", str(ctx.exception))

    def test_raises_when_plugin_config_set_without_plugin_name(self):
        """It raises when plugin_config is set but plugin_name is None."""
        with self.assertRaises(ConfigurationError) as ctx:
            PusherPluginConfig(name="x", plugin_config={"table": "ml_runs"})
        self.assertIn("plugin_name and plugin_config", str(ctx.exception))

    def test_accepts_both_plugin_name_and_plugin_config(self):
        """It does not raise when both plugin_name and plugin_config are set."""
        cfg = PusherPluginConfig(
            name="x",
            plugin_name="hive_plugin",
            plugin_config={"table": "ml_runs"},
        )
        self.assertEqual(cfg.plugin_name, "hive_plugin")

    def test_accepts_neither_plugin_name_nor_plugin_config(self):
        """It does not raise when both plugin_name and plugin_config are None."""
        cfg = PusherPluginConfig(name="x", model_plugin=ModelPluginConfig())
        self.assertIsNone(cfg.plugin_name)
        self.assertIsNone(cfg.plugin_config)


class TestResolvedPluginConfigExtensionPluginMissingConfig(TestCase):
    """Tests for resolved_plugin_config() when plugin_config is not provided."""

    def test_raises_when_extension_plugin_config_is_none(self):
        """It raises ConfigurationError (not None) when plugin_config is missing."""
        cfg = PusherPluginConfig(
            name="x",
            plugin_name="hive_plugin",
            plugin_config={"table": "ml"},
        )
        # Simulate misconfiguration by nulling plugin_config after construction.
        cfg.plugin_config = None
        with self.assertRaises(ConfigurationError) as ctx:
            cfg.resolved_plugin_config()
        self.assertIn("plugin_config is None", str(ctx.exception))
