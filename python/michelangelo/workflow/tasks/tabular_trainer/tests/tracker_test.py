"""Tests for michelangelo.workflow.tasks.tabular_trainer._private.tracker."""

from __future__ import annotations

from unittest import TestCase

from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.tabular_trainer import (
    CometConfig,
    CustomTrackerConfig,
    ExperimentTrackerConfig,
)
from michelangelo.workflow.tasks.tabular_trainer._private.tracker import (
    build_tracker_logger_kwargs,
)

_COMET_FACTORY = (
    "michelangelo.lib.trainer.torch.pytorch_lightning._private.util.build_comet_logger"
)


class TestBuildTrackerLoggerKwargs(TestCase):
    """Tests for build_tracker_logger_kwargs."""

    def test_none_config_returns_no_logger(self):
        """config=None disables tracking."""
        result = build_tracker_logger_kwargs(None)
        self.assertEqual(result, {"logger": None, "logger_kwargs": None})

    def test_empty_config_returns_no_logger(self):
        """ExperimentTrackerConfig() with no tracker set disables tracking."""
        result = build_tracker_logger_kwargs(ExperimentTrackerConfig())
        self.assertEqual(result, {"logger": None, "logger_kwargs": None})

    def test_comet_config_resolves_to_build_comet_logger(self):
        """CometConfig maps to the build_comet_logger dotted path + kwargs."""
        config = ExperimentTrackerConfig(
            tracker=CometConfig(
                api_key="k",
                workspace="ws",
                project_name="proj",
                experiment_name="exp",
                tags=["a", "b"],
            )
        )
        result = build_tracker_logger_kwargs(config)
        self.assertEqual(result["logger"], _COMET_FACTORY)
        self.assertEqual(
            result["logger_kwargs"],
            {
                "api_key": "k",
                "workspace": "ws",
                "project_name": "proj",
                "experiment_name": "exp",
                "tags": ["a", "b"],
            },
        )

    def test_legacy_comet_field_resolves_same_as_tracker_field(self):
        """Legacy comet= promotion produces the same result as tracker=."""
        comet = CometConfig(
            api_key="k", workspace="ws", project_name="proj", experiment_name="exp"
        )
        result = build_tracker_logger_kwargs(ExperimentTrackerConfig(comet=comet))
        self.assertEqual(result["logger"], _COMET_FACTORY)

    def test_custom_tracker_config_resolves_to_factory_fn(self):
        """CustomTrackerConfig maps directly to factory_fn/factory_kwargs."""
        config = ExperimentTrackerConfig(
            tracker=CustomTrackerConfig(
                factory_fn="myproject.loggers.make_wandb_logger",
                factory_kwargs={"project": "ctr-model"},
            )
        )
        result = build_tracker_logger_kwargs(config)
        self.assertEqual(result["logger"], "myproject.loggers.make_wandb_logger")
        self.assertEqual(result["logger_kwargs"], {"project": "ctr-model"})

    def test_custom_tracker_config_default_kwargs(self):
        """No factory_kwargs given returns an empty dict, not None."""
        config = ExperimentTrackerConfig(
            tracker=CustomTrackerConfig(factory_fn="myproject.loggers.make_logger")
        )
        result = build_tracker_logger_kwargs(config)
        self.assertEqual(result["logger_kwargs"], {})

    def test_unsupported_tracker_type_raises(self):
        """A TrackerConfig subclass not handled here raises ConfigurationError."""
        from dataclasses import dataclass

        from michelangelo.workflow.schema.tabular_trainer import TrackerConfig

        @dataclass
        class _FutureTrackerConfig(TrackerConfig):
            pass

        config = ExperimentTrackerConfig(tracker=_FutureTrackerConfig())
        with self.assertRaises(ConfigurationError):
            build_tracker_logger_kwargs(config)
