"""Tests for the resolver helpers in ``_private/util.py``.

Covers ``_resolve_strategy`` / ``_resolve_plugins`` / ``_resolve_logger`` /
``_resolve_callbacks`` — pure, easy-to-test factories that map user-supplied
strategy/plugin/logger/callback inputs to the Ray-Lightning runtime objects.

Some Ray/DeepSpeed classes require a real Ray cluster or GPU driver to
instantiate; those classes are patched at the resolver-module level so the
resolvers can be tested in isolation.
"""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest
from pytorch_lightning.callbacks import Callback, ModelCheckpoint
from pytorch_lightning.loggers import Logger
from ray.train.lightning import (
    RayDDPStrategy,
    RayLightningEnvironment,
)

from michelangelo.lib.trainer.torch.pytorch_lightning._private.util import (
    _resolve_callbacks,
    _resolve_logger,
    _resolve_plugins,
    _resolve_strategy,
)

_UTIL_MODULE = "michelangelo.lib.trainer.torch.pytorch_lightning._private.util"


# -----------------------------------------------------------------------------
# _resolve_strategy
# -----------------------------------------------------------------------------


class TestResolveStrategy:
    """Strategy name / instance → Ray-Lightning Strategy resolution."""

    def test_none_defaults_to_ddp(self):
        """``None`` resolves to :class:`RayDDPStrategy`."""
        assert isinstance(_resolve_strategy(None), RayDDPStrategy)

    @pytest.mark.parametrize("name", ["ddp", "DDP", "Ddp"])
    def test_ddp_string_resolves_case_insensitively(self, name):
        """The string ``"ddp"`` is recognized case-insensitively."""
        assert isinstance(_resolve_strategy(name), RayDDPStrategy)

    @pytest.mark.parametrize("name", ["deepspeed", "DeepSpeed", "DEEPSPEED"])
    def test_deepspeed_string_routes_to_deepspeed_strategy(self, name):
        """The string ``"deepspeed"`` routes to ``RayDeepSpeedStrategy``.

        ``RayDeepSpeedStrategy()`` itself requires a GPU driver to construct;
        we patch the class so we can verify routing without instantiating it.
        """
        with patch(f"{_UTIL_MODULE}.RayDeepSpeedStrategy") as mock_cls:
            result = _resolve_strategy(name)
        mock_cls.assert_called_once_with()
        assert result is mock_cls.return_value

    @pytest.mark.parametrize("name", ["fsdp", "FSDP", "fSdP"])
    def test_fsdp_string_routes_to_fsdp_strategy(self, name):
        """The string ``"fsdp"`` routes to ``RayFSDPStrategy``."""
        with patch(f"{_UTIL_MODULE}.RayFSDPStrategy") as mock_cls:
            result = _resolve_strategy(name)
        mock_cls.assert_called_once_with()
        assert result is mock_cls.return_value

    def test_strategy_instance_passes_through(self):
        """A ready :class:`Strategy` instance is returned unchanged."""
        instance = RayDDPStrategy()
        assert _resolve_strategy(instance) is instance

    def test_unsupported_string_raises_value_error(self):
        """An unknown strategy name raises ``ValueError``."""
        with pytest.raises(ValueError, match="Unsupported strategy"):
            _resolve_strategy("horovod")

    def test_invalid_type_raises_type_error(self):
        """Non-str/Strategy/None inputs raise ``TypeError``."""
        with pytest.raises(TypeError, match="strategy must be"):
            _resolve_strategy(123)

    def test_invalid_strategy_kwargs_type_raises(self):
        """``strategy_kwargs`` must be a dict."""
        with pytest.raises(TypeError, match="strategy_kwargs must be"):
            _resolve_strategy("ddp", strategy_kwargs=["not", "a", "dict"])


# -----------------------------------------------------------------------------
# _resolve_plugins
# -----------------------------------------------------------------------------


class TestResolvePlugins:
    """Plugin resolution always appends ``RayLightningEnvironment``."""

    def test_none_returns_environment_only(self):
        """``None`` yields a list containing just the Ray environment plugin."""
        out = _resolve_plugins(None)
        assert len(out) == 1
        assert isinstance(out[0], RayLightningEnvironment)

    def test_existing_ray_env_not_duplicated(self):
        """If the user already supplied ``RayLightningEnvironment``, no dup is added."""
        env = RayLightningEnvironment()
        out = _resolve_plugins([env])
        assert out == [env]

    def test_invalid_plugin_type_in_list_raises(self):
        """A list with a non-plugin item raises ``TypeError``."""
        with pytest.raises(TypeError, match="All plugins must be instances of"):
            _resolve_plugins([object()])

    def test_invalid_top_level_type_raises(self):
        """A non-str/list/plugin top-level input raises ``TypeError``."""
        with pytest.raises(TypeError, match="plugins must be"):
            _resolve_plugins(123)

    def test_invalid_plugins_kwargs_type_raises(self):
        """``plugins_kwargs`` must be a dict."""
        with pytest.raises(TypeError, match="plugins_kwargs must be"):
            _resolve_plugins("some.path", plugins_kwargs=[1, 2])

    def test_plugins_kwargs_requires_string_path(self):
        """``plugins_kwargs`` only makes sense with a str import path."""
        with pytest.raises(TypeError, match="plugins_kwargs can only be used"):
            _resolve_plugins(RayLightningEnvironment(), plugins_kwargs={"x": 1})


# -----------------------------------------------------------------------------
# _resolve_logger
# -----------------------------------------------------------------------------


class _FakeLogger(Logger):
    """Minimal Logger subclass for tests (avoids instantiating real backends)."""

    @property
    def name(self):
        """Logger display name."""
        return "fake"

    @property
    def version(self):
        """Logger version string."""
        return "0"

    def log_hyperparams(self, params):
        """No-op for tests."""

    def log_metrics(self, metrics, step=None):
        """No-op for tests."""


class TestResolveLogger:
    """Logger resolution covers bool, instance, list, and ``None`` paths."""

    def test_bool_passes_through(self):
        """``True`` / ``False`` are returned as-is (Lightning's default behavior)."""
        assert _resolve_logger(True) is True
        assert _resolve_logger(False) is False

    def test_logger_instance_passes_through(self):
        """A :class:`Logger` instance is returned unchanged."""
        logger = _FakeLogger()
        assert _resolve_logger(logger) is logger

    def test_list_of_loggers_normalized_to_list(self):
        """A tuple/list of loggers becomes a list of loggers."""
        a, b = _FakeLogger(), _FakeLogger()
        out = _resolve_logger((a, b))
        assert out == [a, b]

    def test_list_with_non_logger_raises(self):
        """Lists must contain only :class:`Logger` instances."""
        with pytest.raises(TypeError, match="All elements of logger list"):
            _resolve_logger([_FakeLogger(), object()])

    def test_invalid_top_level_type_raises(self):
        """Non-str/bool/Logger/list top-level inputs raise ``TypeError``."""
        with pytest.raises(TypeError, match="logger must be"):
            _resolve_logger(123)

    def test_none_with_no_comet_returns_none(self):
        """``None`` input + no ``comet_param`` falls back to ``None``."""
        assert _resolve_logger(None) is None

    def test_logger_kwargs_requires_string_logger(self):
        """``logger_kwargs`` only makes sense with a str import path."""
        with pytest.raises(TypeError, match="logger_kwargs can only be used"):
            _resolve_logger(True, logger_kwargs={"x": 1})

    def test_invalid_comet_param_type_raises(self):
        """``comet_param`` must be a dict when set."""
        with pytest.raises(TypeError, match="comet_param must be"):
            _resolve_logger(None, comet_param="not-a-dict")


# -----------------------------------------------------------------------------
# _resolve_callbacks
# -----------------------------------------------------------------------------


class _FakeCallback(Callback):
    """Minimal Callback subclass for tests."""


@pytest.fixture
def patched_ray_report_callbacks():
    """Patch the Ray report callback classes so resolvers don't touch Ray runtime.

    ``RayTrainReportCallback.__init__`` accesses ``ray.train.get_context()``,
    which fails without an active Ray Train session. We patch the classes so
    they construct trivially and verify routing via ``isinstance`` against the
    patched class.
    """
    with (
        patch(f"{_UTIL_MODULE}.RayTrainReportCallback") as mock_default,
        patch(f"{_UTIL_MODULE}.RayTrainReportPerNodeCallback") as mock_per_node,
    ):
        yield mock_default, mock_per_node


class TestResolveCallbacks:
    """Callback resolution always appends a Ray report callback."""

    def test_none_appends_default_report_callback(self, patched_ray_report_callbacks):
        """``None`` callbacks → just the per-rank-0 report callback."""
        mock_default, _ = patched_ray_report_callbacks
        callbacks, has_ckpt = _resolve_callbacks(None)
        assert len(callbacks) == 1
        assert callbacks[0] is mock_default.return_value
        assert has_ckpt is False

    def test_user_list_preserved_and_report_appended(
        self, patched_ray_report_callbacks
    ):
        """User callbacks are kept and the Ray report callback is appended."""
        mock_default, _ = patched_ray_report_callbacks
        user_cb = _FakeCallback()
        callbacks, has_ckpt = _resolve_callbacks([user_cb])
        assert callbacks[0] is user_cb
        assert callbacks[-1] is mock_default.return_value
        assert has_ckpt is False

    def test_model_checkpoint_present_sets_has_ckpt(self, patched_ray_report_callbacks):
        """Presence of a :class:`ModelCheckpoint` flips the ``has_ckpt`` flag."""
        mc = ModelCheckpoint()
        callbacks, has_ckpt = _resolve_callbacks([mc])
        assert has_ckpt is True
        assert callbacks[0] is mc

    def test_deepspeed_strategy_triggers_per_node_callback(
        self, patched_ray_report_callbacks
    ):
        """A DeepSpeed strategy triggers the per-node Ray report callback."""
        from ray.train.lightning import RayDeepSpeedStrategy

        _, mock_per_node = patched_ray_report_callbacks
        strategy = MagicMock(spec=RayDeepSpeedStrategy)
        callbacks, _ = _resolve_callbacks(None, strategy=strategy)
        assert callbacks[-1] is mock_per_node.return_value

    def test_fsdp_strategy_triggers_per_node_callback(
        self, patched_ray_report_callbacks
    ):
        """An FSDP strategy also triggers the per-node Ray report callback."""
        from ray.train.lightning import RayFSDPStrategy

        _, mock_per_node = patched_ray_report_callbacks
        strategy = MagicMock(spec=RayFSDPStrategy)
        callbacks, _ = _resolve_callbacks(None, strategy=strategy)
        assert callbacks[-1] is mock_per_node.return_value

    def test_explicit_per_node_kwargs_triggers_per_node_callback(
        self, patched_ray_report_callbacks
    ):
        """Explicit ``per_node_callback_kwargs`` also selects per-node reporting."""
        _, mock_per_node = patched_ray_report_callbacks
        callbacks, _ = _resolve_callbacks(None, per_node_callback_kwargs={})
        assert callbacks[-1] is mock_per_node.return_value

    def test_invalid_top_level_type_raises(self):
        """A non-str/Callback/list input raises ``TypeError``."""
        with pytest.raises(TypeError, match="callbacks must be"):
            _resolve_callbacks(123)

    def test_invalid_callback_kwargs_raises(self):
        """``callback_kwargs`` must be a dict."""
        with pytest.raises(TypeError, match="callback_kwargs must be"):
            _resolve_callbacks(None, callback_kwargs=["not", "a", "dict"])

    def test_invalid_per_node_callback_kwargs_raises(self):
        """``per_node_callback_kwargs`` must be a dict."""
        with pytest.raises(TypeError, match="per_node_callback_kwargs must be"):
            _resolve_callbacks(None, per_node_callback_kwargs=["nope"])

    def test_list_with_non_callback_raises(self):
        """A list with a non-Callback item raises ``TypeError``."""
        with pytest.raises(TypeError, match="All callbacks must be"):
            _resolve_callbacks([object()])
