"""Tests for the push() dispatch function."""

from __future__ import annotations

import tempfile
from typing import Any
from unittest import TestCase

from michelangelo.lib.artifact_manager.storage_backend import LocalStorageBackend
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.pusher import (
    PusherConfig,
    PusherPluginConfig,
)
from michelangelo.workflow.tasks.pusher import (
    ArtifactNotFoundError,
    PluginRegistry,
    PusherPluginError,
    push,
)
from michelangelo.workflow.tasks.pusher.plugins.base import PusherPluginBase
from michelangelo.workflow.variables.types import (
    AssembledModel,
    ModelArtifact,
    PusherResult,
)

# ---------------------------------------------------------------------------
# Mock plugin helpers
# ---------------------------------------------------------------------------


def _fake_plugin_class(
    return_value: dict[str, Any] | None = None,
    raises: Exception | None = None,
) -> type[PusherPluginBase]:
    """Return a PusherPluginBase subclass whose execute() is scripted."""
    rv = return_value or {}
    exc = raises

    class _FakePlugin(PusherPluginBase):
        def execute(self) -> dict[str, Any]:
            """Execute the scripted fake plugin."""
            if exc is not None:
                raise exc
            return rv

    return _FakePlugin


def _registry(*entries: tuple) -> PluginRegistry:
    """Build a fresh PluginRegistry with the given (name, cls, type) entries."""
    reg = PluginRegistry()
    for name, plugin_cls, artifact_type in entries:
        reg.register(name, plugin_cls, artifact_type)
    return reg


def _config(*items: PusherPluginConfig) -> PusherConfig:
    """Wrap items in a PusherConfig."""
    return PusherConfig(items=list(items))


def _item(name: str, plugin_name: str) -> PusherPluginConfig:
    """Build a PusherPluginConfig pointing at a custom plugin_name."""
    return PusherPluginConfig(name=name, plugin_name=plugin_name, plugin_config={})


def _assembled(path: str = "/tmp/raw") -> AssembledModel:
    """Build a minimal AssembledModel."""
    return AssembledModel(raw_model=ModelArtifact(path=path))


def _storage() -> LocalStorageBackend:
    """Return a temporary LocalStorageBackend backed by a self-cleaning temp dir."""
    tmp = tempfile.mkdtemp()
    return LocalStorageBackend(tmp)


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


class TestPushSingleSuccess(TestCase):
    """Test 1: single plugin succeeds."""

    def test_returns_one_successful_result(self) -> None:
        """It returns a single PusherResult with success=True and plugin output."""
        fake_plugin = _fake_plugin_class(return_value={"uri": "s3://bucket/key"})
        reg = _registry(("fake_plugin", fake_plugin, AssembledModel))

        results = push(
            config=_config(_item("model", "fake_plugin")),
            artifacts={"model": _assembled()},
            storage_backend=_storage(),
            registry=reg,
        )

        self.assertEqual(len(results), 1)
        self.assertIsInstance(results[0], PusherResult)
        self.assertTrue(results[0].success)
        self.assertEqual(results[0].name, "model")
        self.assertEqual(results[0].plugin, "fake_plugin")
        self.assertEqual(results[0].value, {"uri": "s3://bucket/key"})
        self.assertIsNone(results[0].error)


class TestPushMultipleSuccess(TestCase):
    """Test 2: multiple items, all succeed, returned in config order."""

    def test_returns_results_in_order(self) -> None:
        """It returns one result per item in the same order as config.items."""
        fake_plugin = _fake_plugin_class()
        reg = _registry(
            ("plugin_a", fake_plugin, AssembledModel),
            ("plugin_b", fake_plugin, AssembledModel),
        )
        artifacts = {"first": _assembled(), "second": _assembled()}
        cfg = _config(_item("first", "plugin_a"), _item("second", "plugin_b"))

        results = push(
            config=cfg, artifacts=artifacts, storage_backend=_storage(), registry=reg
        )

        self.assertEqual(len(results), 2)
        self.assertEqual(results[0].name, "first")
        self.assertEqual(results[1].name, "second")
        self.assertTrue(all(r.success for r in results))


class TestPushArtifactNotFound(TestCase):
    """Test 3: artifact key absent from artifacts dict."""

    def test_raises_artifact_not_found_error(self) -> None:
        """It raises ArtifactNotFoundError when an artifact name is missing."""
        fake_plugin = _fake_plugin_class()
        reg = _registry(("fp", fake_plugin, AssembledModel))

        with self.assertRaises(ArtifactNotFoundError):
            push(
                config=_config(_item("missing", "fp")),
                artifacts={"other": _assembled()},
                storage_backend=_storage(),
                registry=reg,
            )


class TestPushUnknownPlugin(TestCase):
    """Test 4: plugin name not in registry."""

    def test_raises_configuration_error(self) -> None:
        """It raises ConfigurationError when the plugin name is not registered."""
        reg = PluginRegistry()  # empty

        with self.assertRaises(ConfigurationError):
            push(
                config=_config(_item("model", "no_such_plugin")),
                artifacts={"model": _assembled()},
                storage_backend=_storage(),
                registry=reg,
            )


class TestPushTypeMismatch(TestCase):
    """Test 5: artifact type does not match registered expected type."""

    def test_raises_configuration_error_on_type_mismatch(self) -> None:
        """It raises ConfigurationError when the artifact type is wrong."""
        fake_plugin = _fake_plugin_class()
        reg = _registry(("typed_plugin", fake_plugin, AssembledModel))

        with self.assertRaisesRegex(ConfigurationError, r"type|AssembledModel"):
            push(
                config=_config(_item("report", "typed_plugin")),
                artifacts={"report": {"not": "an AssembledModel"}},
                storage_backend=_storage(),
                registry=reg,
            )


class TestPushFailFastTrue(TestCase):
    """Test 6: fail_fast=True — first failure raises, second plugin not called."""

    def test_raises_and_skips_remaining(self) -> None:
        """It raises PusherPluginError and never invokes the second plugin."""
        fail_plugin = _fake_plugin_class(raises=RuntimeError("boom"))
        call_log: list[str] = []

        class _TrackingPlugin(PusherPluginBase):
            def execute(self) -> dict[str, Any]:
                """Record invocation and return empty dict."""
                call_log.append("second")
                return {}

        reg = _registry(
            ("fail_plugin", fail_plugin, AssembledModel),
            ("ok_plugin", _TrackingPlugin, AssembledModel),
        )
        artifacts = {"first": _assembled(), "second": _assembled()}
        cfg = _config(_item("first", "fail_plugin"), _item("second", "ok_plugin"))

        with self.assertRaises(PusherPluginError):
            push(
                config=cfg,
                artifacts=artifacts,
                storage_backend=_storage(),
                registry=reg,
                fail_fast=True,
            )

        self.assertNotIn("second", call_log)


class TestPushFailFastFalse(TestCase):
    """Test 7: fail_fast=False — all items run; failure captured in result."""

    def test_all_run_failure_in_result(self) -> None:
        """It runs all items and records the error on the failing result."""
        fail_plugin = _fake_plugin_class(raises=RuntimeError("oops"))
        ok_plugin = _fake_plugin_class(return_value={"done": True})
        reg = _registry(
            ("fail_plugin", fail_plugin, AssembledModel),
            ("ok_plugin", ok_plugin, AssembledModel),
        )
        artifacts = {"first": _assembled(), "second": _assembled()}
        cfg = _config(_item("first", "fail_plugin"), _item("second", "ok_plugin"))

        results = push(
            config=cfg,
            artifacts=artifacts,
            storage_backend=_storage(),
            registry=reg,
            fail_fast=False,
        )

        self.assertEqual(len(results), 2)
        self.assertFalse(results[0].success)
        self.assertIsNotNone(results[0].error)
        self.assertIn("oops", results[0].error)
        self.assertTrue(results[1].success)


class TestPushOnErrorCallback(TestCase):
    """Test 8: on_error callback invoked with (artifact_name, plugin_name, exc)."""

    def test_on_error_receives_correct_args(self) -> None:
        """It calls on_error with the artifact name, plugin name, and exception."""
        err = ValueError("fail")
        fail_plugin = _fake_plugin_class(raises=err)
        reg = _registry(("fp", fail_plugin, AssembledModel))

        calls: list[tuple] = []

        def on_error(name: str, plugin: str, exc: Exception) -> None:
            calls.append((name, plugin, exc))

        with self.assertRaises(PusherPluginError):
            push(
                config=_config(_item("art", "fp")),
                artifacts={"art": _assembled()},
                storage_backend=_storage(),
                registry=reg,
                on_error=on_error,
            )

        self.assertEqual(len(calls), 1)
        self.assertEqual(calls[0][0], "art")
        self.assertEqual(calls[0][1], "fp")
        self.assertIs(calls[0][2], err)


class TestPushChildRegistry(TestCase):
    """Test 9: custom child registry resolves its own plugin correctly."""

    def test_child_registry_overrides_parent(self) -> None:
        """It resolves the child-registered plugin, not the parent's version."""
        base_plugin = _fake_plugin_class(return_value={"source": "parent"})
        override_plugin = _fake_plugin_class(return_value={"source": "child"})

        parent = _registry(("my_plugin", base_plugin, AssembledModel))
        child = parent.extend()
        child.register("my_plugin", override_plugin, AssembledModel)

        results = push(
            config=_config(_item("art", "my_plugin")),
            artifacts={"art": _assembled()},
            storage_backend=_storage(),
            registry=child,
        )

        self.assertTrue(results[0].success)
        self.assertEqual(results[0].value["source"], "child")


class TestPushOnErrorCallbackRaises(TestCase):
    """Test 10: on_error callback that raises is suppressed; push still proceeds."""

    def test_raising_on_error_is_suppressed(self) -> None:
        """It suppresses exceptions from on_error and still raises PusherPluginError."""
        fail_plugin = _fake_plugin_class(raises=RuntimeError("plugin-fail"))
        reg = _registry(("fp", fail_plugin, AssembledModel))

        def bad_callback(name: str, plugin: str, exc: Exception) -> None:
            raise ValueError("callback-fail")

        with self.assertRaises(PusherPluginError):
            push(
                config=_config(_item("art", "fp")),
                artifacts={"art": _assembled()},
                storage_backend=_storage(),
                registry=reg,
                on_error=bad_callback,
                fail_fast=True,
            )

    def test_raising_on_error_does_not_affect_fail_fast_false(self) -> None:
        """It suppresses callback exception and continues processing remaining items."""
        fail_plugin = _fake_plugin_class(raises=RuntimeError("boom"))
        ok_plugin = _fake_plugin_class(return_value={"ok": True})
        reg = _registry(
            ("fail_plugin", fail_plugin, AssembledModel),
            ("ok_plugin", ok_plugin, AssembledModel),
        )

        def bad_callback(name: str, plugin: str, exc: Exception) -> None:
            raise ValueError("callback-boom")

        results = push(
            config=_config(_item("first", "fail_plugin"), _item("second", "ok_plugin")),
            artifacts={"first": _assembled(), "second": _assembled()},
            storage_backend=_storage(),
            registry=reg,
            on_error=bad_callback,
            fail_fast=False,
        )

        self.assertEqual(len(results), 2)
        self.assertFalse(results[0].success)
        self.assertTrue(results[1].success)


class TestPushDefaultStorageBackend(TestCase):
    """Test 11: storage_backend=None raises ConfigurationError."""

    def test_no_storage_backend_raises(self) -> None:
        """It raises ConfigurationError when storage_backend is omitted."""
        fake_plugin = _fake_plugin_class(return_value={"ok": True})
        reg = _registry(("fp", fake_plugin, AssembledModel))

        with self.assertRaisesRegex(ConfigurationError, "storage_backend"):
            push(
                config=_config(_item("art", "fp")),
                artifacts={"art": _assembled()},
                registry=reg,
                # storage_backend intentionally omitted
            )


class TestPushDefaultRegistry(TestCase):
    """Test 12: registry=None uses default_registry with built-in plugins."""

    def test_default_registry_contains_builtin_plugins(self) -> None:
        """It populates default_registry with the three built-in plugin names."""
        from michelangelo.workflow.tasks.pusher import (  # noqa: F401 — triggers registration
            push,
        )
        from michelangelo.workflow.tasks.pusher.registry import default_registry

        names = default_registry.registered_names()
        self.assertIn("model_plugin", names)
        self.assertIn("dataset_plugin", names)
        self.assertIn("eval_report_plugin", names)

    def test_default_registry_used_when_registry_omitted(self) -> None:
        """It raises from the registered plugin when registry is omitted."""
        from michelangelo.workflow.schema.exceptions import (
            ConfigurationError as SchemaConfigError,
        )
        from michelangelo.workflow.schema.pusher import (
            ModelPluginConfig,
            PusherPluginConfig,
        )

        cfg = PusherConfig(
            items=[
                PusherPluginConfig(
                    name="model",
                    model_plugin=ModelPluginConfig(model_name="test-model"),
                )
            ]
        )

        # Without registry_client, ModelPusherPlugin raises ConfigurationError
        # about a missing registry client — NOT about an unknown plugin name.
        # This distinguishes "plugin found and reached" from "plugin not registered".
        with self.assertRaisesRegex(SchemaConfigError, "registry client"):
            push(
                config=cfg,
                artifacts={"model": _assembled()},
                storage_backend=_storage(),
                # registry intentionally omitted — uses default_registry
                # registry_client intentionally omitted — causes plugin validation error
            )


class TestPushUntypedPlugin(TestCase):
    """Test 13: plugin registered with artifact_type=None accepts any artifact."""

    def test_untyped_plugin_accepts_any_artifact(self) -> None:
        """It skips isinstance check when artifact_type is None."""
        fake_plugin = _fake_plugin_class(return_value={"ok": True})
        reg = PluginRegistry()
        reg.register("untyped_plugin", fake_plugin, None)  # no type constraint

        results = push(
            config=_config(_item("anything", "untyped_plugin")),
            artifacts={"anything": {"arbitrary": "dict"}},  # not an AssembledModel
            storage_backend=_storage(),
            registry=reg,
        )

        self.assertTrue(results[0].success)


class TestPushTupleExpectedType(TestCase):
    """Test 14: plugin registered with a tuple of types accepts any matching type."""

    def test_tuple_type_accepts_matching_types(self) -> None:
        """It passes isinstance check when artifact matches any type in the tuple."""
        fake_plugin = _fake_plugin_class(return_value={"ok": True})
        reg = PluginRegistry()
        reg.register("multi_plugin", fake_plugin, (AssembledModel, dict))

        for artifact in [_assembled(), {"raw": "dict"}]:
            results = push(
                config=_config(_item("art", "multi_plugin")),
                artifacts={"art": artifact},
                storage_backend=_storage(),
                registry=reg,
            )
            self.assertTrue(results[0].success)

    def test_tuple_type_rejects_non_matching_type(self) -> None:
        """It raises ConfigurationError when artifact doesn't match any tuple type."""
        fake_plugin = _fake_plugin_class()
        reg = PluginRegistry()
        reg.register("multi_plugin", fake_plugin, (AssembledModel, dict))

        with self.assertRaises(ConfigurationError):
            push(
                config=_config(_item("art", "multi_plugin")),
                artifacts={"art": [1, 2, 3]},  # list — not in (AssembledModel, dict)
                storage_backend=_storage(),
                registry=reg,
            )


class TestPushOnErrorCallbackExceptionSuppressed(TestCase):
    """Test 15: exceptions raised inside on_error are suppressed, not re-raised."""

    def test_on_error_exception_does_not_mask_plugin_error(self) -> None:
        """When on_error itself raises, PusherPluginError still propagates."""
        fail_plugin = _fake_plugin_class(raises=RuntimeError("plugin boom"))

        def _bad_callback(name: str, plugin: str, exc: Exception) -> None:
            raise RuntimeError("callback boom")

        reg = _registry(("fp", fail_plugin, AssembledModel))

        with self.assertRaises(PusherPluginError):
            push(
                config=_config(_item("art", "fp")),
                artifacts={"art": _assembled()},
                storage_backend=_storage(),
                registry=reg,
                fail_fast=True,
                on_error=_bad_callback,
            )

    def test_on_error_exception_suppressed_in_fail_fast_false(self) -> None:
        """In fail_fast=False mode, a raising on_error still records the result."""
        fail_plugin = _fake_plugin_class(raises=RuntimeError("plugin boom"))
        callback_raised: list[bool] = []

        def _bad_callback(name: str, plugin: str, exc: Exception) -> None:
            callback_raised.append(True)
            raise RuntimeError("callback boom")

        reg = _registry(("fp", fail_plugin, AssembledModel))

        results = push(
            config=_config(_item("art", "fp")),
            artifacts={"art": _assembled()},
            storage_backend=_storage(),
            registry=reg,
            fail_fast=False,
            on_error=_bad_callback,
        )

        self.assertTrue(callback_raised)
        self.assertFalse(results[0].success)
        self.assertIn("plugin boom", results[0].error)
