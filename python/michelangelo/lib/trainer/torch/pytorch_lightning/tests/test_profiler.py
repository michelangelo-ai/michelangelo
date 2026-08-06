"""Tests for the profiler subsystem in ``_private/util.py``.

Covers step-count estimation, ``pytorch`` profiler construction, schedule
defaults and validation, config resolution, and the post-``fit``
``profiler_sink`` hook.

``ray.train.get_context`` is mocked throughout so the helpers run without a
Ray cluster.
"""

from __future__ import annotations

import os
from unittest.mock import MagicMock, patch

import pytest

# The util module imports ray / torch / pytorch_lightning at import time. Skip
# cleanly when those optional heavy dependencies are unavailable.
pytest.importorskip("ray")
pytest.importorskip("torch")
pl = pytest.importorskip("pytorch_lightning")

from pytorch_lightning.profilers import (  # noqa: E402
    AdvancedProfiler,
    PyTorchProfiler,
    SimpleProfiler,
    XLAProfiler,
)

from michelangelo.lib._internal.errors import UserInputError  # noqa: E402
from michelangelo.lib.trainer.torch.pytorch_lightning._private.util import (  # noqa: E402
    _build_profiler,
    _compute_default_schedule,
    _compute_steps_per_epoch,
    _maybe_export_profiler_results,
    _profiler_output,
    _resolve_profiler,
    _validate_profiler_schedule,
    comet_profiler_sink,
    mlflow_profiler_sink,
)

_UTIL_MODULE = "michelangelo.lib.trainer.torch.pytorch_lightning._private.util"


@pytest.fixture
def in_tmp_cwd(tmp_path, monkeypatch):
    """Run the test with cwd set to a temp dir so ``profiler_logs`` is isolated."""
    monkeypatch.chdir(tmp_path)
    return tmp_path


@pytest.fixture
def patched_ray(monkeypatch):
    """Patch ``ray`` in the util module with world rank 0 / local rank 0."""
    with patch(f"{_UTIL_MODULE}.ray") as ray_mod:
        ctx = ray_mod.train.get_context.return_value
        ctx.get_world_rank.return_value = 0
        ctx.get_local_rank.return_value = 0
        yield ctx


# -----------------------------------------------------------------------------
# _compute_steps_per_epoch
# -----------------------------------------------------------------------------


class TestComputeStepsPerEpoch:
    """Steps-per-epoch estimation and Lightning's batch-limiting arguments."""

    def test_none_rows_returns_none(self):
        """An unknown row count yields an unknown step count."""
        assert _compute_steps_per_epoch({}, None, batch_size=4) is None

    @pytest.mark.parametrize(
        ("rows", "batch_size", "world_size", "expected"),
        [
            (100, 10, 1, 10),
            (100, 10, 2, 5),
            (101, 10, 1, 11),  # ceil
            (100, 3, 2, 17),  # ceil(100/2/3)
            (0, 10, 1, 0),
        ],
    )
    def test_basic_estimate(self, rows, batch_size, world_size, expected):
        """The base estimate is ``ceil(rows / world_size / batch_size)``."""
        assert _compute_steps_per_epoch({}, rows, batch_size, world_size) == expected

    def test_limit_train_batches_float_scales(self):
        """A float ``limit_train_batches`` scales the epoch proportionally."""
        assert _compute_steps_per_epoch({"limit_train_batches": 0.5}, 100, 10, 1) == 5

    def test_limit_train_batches_float_one_is_noop(self):
        """``1.0`` keeps the full epoch."""
        assert _compute_steps_per_epoch({"limit_train_batches": 1.0}, 100, 10, 1) == 10

    def test_limit_train_batches_int_caps(self):
        """An int ``limit_train_batches`` caps but never raises the estimate."""
        assert _compute_steps_per_epoch({"limit_train_batches": 3}, 100, 10, 1) == 3
        assert _compute_steps_per_epoch({"limit_train_batches": 50}, 100, 10, 1) == 10

    @pytest.mark.parametrize("bad", [1.5, -0.5, 0, -3, "half"])
    def test_invalid_limit_train_batches_raises(self, bad):
        """Values Lightning would reject raise ``ValueError``."""
        with pytest.raises(ValueError, match="limit_train_batches"):
            _compute_steps_per_epoch({"limit_train_batches": bad}, 100, 10, 1)

    def test_max_steps_caps_estimate(self):
        """``max_steps`` below the epoch length wins."""
        assert _compute_steps_per_epoch({"max_steps": 4}, 100, 10, 1) == 4

    def test_max_steps_above_epoch_is_ignored(self):
        """``max_steps`` above the epoch length leaves the estimate alone."""
        assert _compute_steps_per_epoch({"max_steps": 999}, 100, 10, 1) == 10

    def test_max_steps_scaled_by_accumulate_grad_batches(self):
        """``max_steps`` counts optimizer steps, so it scales by accumulation."""
        assert (
            _compute_steps_per_epoch(
                {"max_steps": 2, "accumulate_grad_batches": 3}, 100, 10, 1
            )
            == 6
        )

    def test_max_steps_disabled_sentinel_is_ignored(self):
        """Lightning's ``max_steps=-1`` sentinel means "no cap"."""
        assert _compute_steps_per_epoch({"max_steps": -1}, 100, 10, 1) == 10

    def test_limit_and_max_steps_applied_in_order(self):
        """``limit_train_batches`` applies first, then ``max_steps`` caps it."""
        assert (
            _compute_steps_per_epoch(
                {"limit_train_batches": 0.5, "max_steps": 3}, 100, 10, 1
            )
            == 3
        )

    @pytest.mark.parametrize("bad", [0, -1, 2.5])
    def test_invalid_accumulate_grad_batches_raises(self, bad):
        """A non-positive or non-int accumulation factor raises."""
        with pytest.raises(ValueError, match="accumulate_grad_batches"):
            _compute_steps_per_epoch(
                {"max_steps": 2, "accumulate_grad_batches": bad}, 100, 10, 1
            )

    @pytest.mark.parametrize("batch_size", [0, -4])
    def test_invalid_batch_size_raises(self, batch_size):
        """A non-positive batch size raises."""
        with pytest.raises(ValueError, match="batch_size"):
            _compute_steps_per_epoch({}, 100, batch_size, 1)

    @pytest.mark.parametrize("world_size", [0, -2])
    def test_invalid_world_size_raises(self, world_size):
        """A non-positive world size raises."""
        with pytest.raises(ValueError, match="world_size"):
            _compute_steps_per_epoch({}, 100, 10, world_size)


# -----------------------------------------------------------------------------
# _compute_default_schedule
# -----------------------------------------------------------------------------


class TestComputeDefaultSchedule:
    """The documented default schedule for each epoch length."""

    @pytest.mark.parametrize("steps", [None, 0, 1, -5])
    def test_degenerate_epochs_profile_one_step(self, steps):
        """Unknown or single-step epochs profile exactly one step immediately."""
        assert _compute_default_schedule(steps) == {
            "skip_first": 0,
            "wait": 0,
            "warmup": 0,
            "active": 1,
            "repeat": 1,
        }

    def test_two_steps_waits_one(self):
        """Two steps wait one, then profile one."""
        assert _compute_default_schedule(2) == {
            "skip_first": 0,
            "wait": 1,
            "warmup": 0,
            "active": 1,
            "repeat": 1,
        }

    def test_five_steps_shrinks_active(self):
        """Five steps fit a full warmup but a shortened active window."""
        assert _compute_default_schedule(5) == {
            "skip_first": 0,
            "wait": 1,
            "warmup": 2,
            "active": 2,
            "repeat": 1,
        }

    @pytest.mark.parametrize("steps", [7, 30, 300])
    def test_long_epochs_use_full_windows(self, steps):
        """From 7 steps up, warmup and active reach their fixed sizes."""
        assert _compute_default_schedule(steps) == {
            "skip_first": 0,
            "wait": steps // 3,
            "warmup": 2,
            "active": 3,
            "repeat": 1,
        }

    @pytest.mark.parametrize("steps", range(1, 60))
    def test_default_schedule_always_fits_the_epoch(self, steps):
        """The default schedule never exceeds the epoch it was derived from."""
        _validate_profiler_schedule(_compute_default_schedule(steps), steps)


# -----------------------------------------------------------------------------
# _validate_profiler_schedule
# -----------------------------------------------------------------------------


class TestValidateProfilerSchedule:
    """Range and per-epoch bounds on a user-supplied schedule."""

    def test_valid_schedule_passes(self):
        """A schedule that fits the epoch is accepted."""
        _validate_profiler_schedule(
            {"skip_first": 1, "wait": 2, "warmup": 2, "active": 3, "repeat": 1}, 20
        )

    def test_empty_schedule_uses_defaults(self):
        """Omitted fields fall back to defaults that pass validation."""
        _validate_profiler_schedule({}, 10)

    def test_unknown_steps_per_epoch_skips_bounds_check(self):
        """With an unknown epoch length only the field ranges are enforced."""
        _validate_profiler_schedule({"wait": 10_000, "active": 5}, None)

    @pytest.mark.parametrize("field", ["skip_first", "wait", "warmup"])
    def test_negative_fields_raise(self, field):
        """Negative wait-style fields are rejected."""
        with pytest.raises(UserInputError, match=field):
            _validate_profiler_schedule({field: -1}, 100)

    @pytest.mark.parametrize("active", [0, -1, 11, 100])
    def test_active_out_of_range_raises(self, active):
        """``active`` must land in ``[1, 10]``."""
        with pytest.raises(UserInputError, match="active"):
            _validate_profiler_schedule({"active": active}, 1000)

    @pytest.mark.parametrize("repeat", [0, -1, 4, 50])
    def test_repeat_out_of_range_raises(self, repeat):
        """``repeat`` must land in ``[1, 3]``."""
        with pytest.raises(UserInputError, match="repeat"):
            _validate_profiler_schedule({"repeat": repeat}, 1000)

    def test_schedule_longer_than_epoch_raises_with_formula(self):
        """Overrunning the epoch raises and explains the arithmetic."""
        with pytest.raises(UserInputError) as excinfo:
            _validate_profiler_schedule(
                {"skip_first": 1, "wait": 3, "warmup": 2, "active": 3, "repeat": 2}, 10
            )
        message = str(excinfo.value)
        # (3 + 2 + 3) * 2 + 1 = 17 steps requested against an epoch of 10.
        assert "17 steps" in message
        assert "only has 10 steps" in message

    def test_schedule_exactly_filling_epoch_passes(self):
        """A schedule consuming the whole epoch is on the allowed side."""
        _validate_profiler_schedule(
            {"skip_first": 0, "wait": 2, "warmup": 2, "active": 3, "repeat": 2}, 14
        )


# -----------------------------------------------------------------------------
# _build_profiler
# -----------------------------------------------------------------------------


class TestBuildProfiler:
    """Construction of the ``pytorch`` profiler and its config validation."""

    def test_none_config_builds_nothing(self):
        """A ``None`` config yields no profiler and no logs path."""
        assert _build_profiler(None) == (None, "")

    def test_shorthand_string(self, in_tmp_cwd, patched_ray):
        """The ``pytorch`` shorthand builds a ``PyTorchProfiler`` with default kwargs."""
        profiler, logs_path = _build_profiler("pytorch")
        assert isinstance(profiler, PyTorchProfiler)
        assert logs_path == os.path.join(str(in_tmp_cwd), "profiler_logs")
        assert os.path.isdir(logs_path)

    def test_logs_path_and_filename_carry_world_rank(self, in_tmp_cwd, patched_ray):
        """The trace filename embeds the world rank; Lightning adds local rank."""
        patched_ray.get_world_rank.return_value = 3
        profiler, _ = _build_profiler("pytorch")
        assert profiler.filename == "profile-world-rank-3-local-rank"

    def test_pytorch_profiler_gets_default_schedule(self, in_tmp_cwd, patched_ray):
        """The ``pytorch`` key builds a ``PyTorchProfiler`` with a schedule."""
        profiler, logs_path = _build_profiler({"pytorch": {}}, steps_per_epoch=30)
        assert isinstance(profiler, PyTorchProfiler)
        assert profiler.dirpath == logs_path
        assert profiler._schedule is not None

    def test_dict_config_forwards_kwargs(self, in_tmp_cwd, patched_ray):
        """Sub-config kwargs reach the ``PyTorchProfiler`` constructor."""
        profiler, _ = _build_profiler({"pytorch": {"row_limit": 5}}, steps_per_epoch=30)
        assert isinstance(profiler, PyTorchProfiler)
        assert profiler._row_limit == 5

    def test_pytorch_profiler_validates_explicit_schedule(
        self, in_tmp_cwd, patched_ray
    ):
        """An explicit schedule that overruns the epoch is rejected."""
        with pytest.raises(UserInputError, match="only has 4 steps"):
            _build_profiler(
                {"pytorch": {"schedule": {"wait": 5, "warmup": 2, "active": 3}}},
                steps_per_epoch=4,
            )

    def test_pytorch_profiler_resolves_on_trace_ready(self, in_tmp_cwd, patched_ray):
        """A dotted ``on_trace_ready`` path is resolved via ``get_module_attr``."""
        handler = MagicMock(name="handler")
        with patch(f"{_UTIL_MODULE}.get_module_attr", return_value=handler) as resolver:
            _build_profiler(
                {"pytorch": {"on_trace_ready": "some.module.handler"}},
                steps_per_epoch=30,
            )
        resolver.assert_called_once_with("some.module.handler")

    def test_pytorch_sub_config_is_not_mutated(self, in_tmp_cwd, patched_ray):
        """The caller's ``pytorch`` dict survives construction unchanged."""
        sub_config = {"schedule": {"wait": 1, "warmup": 1, "active": 1}}
        config = {"pytorch": sub_config}
        _build_profiler(config, steps_per_epoch=30)
        assert sub_config == {"schedule": {"wait": 1, "warmup": 1, "active": 1}}

    def test_no_profiler_selected_raises(self, in_tmp_cwd, patched_ray):
        """A dict that selects nothing is a user config error."""
        with pytest.raises(UserInputError, match="None were set"):
            _build_profiler({"upload_profiler_results": False})

    def test_multiple_profilers_selected_raises(self, in_tmp_cwd, patched_ray):
        """A dict that selects two profiler flavors is a user config error.

        Only ``pytorch`` is a supported flavor today, so this exercises the
        general N-way selector (kept generic so a second flavor is additive,
        see ``_PROFILER_SHORTHANDS``'s docstring) against a patched-in second
        name rather than a real second profiler.
        """
        with (
            patch(f"{_UTIL_MODULE}._PROFILER_SHORTHANDS", ("pytorch", "other")),
            pytest.raises(UserInputError, match="Multiple were set"),
        ):
            _build_profiler({"pytorch": {}, "other": {}})

    def test_upload_profiler_results_is_not_a_profiler_selection(
        self, in_tmp_cwd, patched_ray
    ):
        """``upload_profiler_results`` is metadata and never reaches a constructor."""
        profiler, _ = _build_profiler({"pytorch": {}, "upload_profiler_results": False})
        assert isinstance(profiler, PyTorchProfiler)

    def test_invalid_shorthand_raises(self):
        """An unrecognized shorthand string raises ``ValueError``."""
        with pytest.raises(ValueError, match="Invalid profiler type"):
            _build_profiler("nonsense")

    @pytest.mark.parametrize("bad", [42, ["pytorch"], object()])
    def test_invalid_config_type_raises(self, bad):
        """A config that is neither str, dict, nor ``None`` raises ``TypeError``."""
        with pytest.raises(TypeError, match="must be a str, dict, or None"):
            _build_profiler(bad)


# -----------------------------------------------------------------------------
# _resolve_profiler
# -----------------------------------------------------------------------------


class TestResolveProfiler:
    """Config resolution, including the ``upload_profiler_results`` opt-out."""

    def test_none_config_resolves_to_nothing(self):
        """No profiler config means no profiler and no export."""
        assert _resolve_profiler(None, {}, 100, 8, 1, 0) == (None, "", False)

    def test_shorthand_defaults_to_uploading(self, in_tmp_cwd, patched_ray):
        """A shorthand string cannot opt out, so export defaults to on."""
        profiler, logs_path, upload = _resolve_profiler("pytorch", {}, 100, 8, 1, 0)
        assert isinstance(profiler, PyTorchProfiler)
        assert logs_path.endswith("profiler_logs")
        assert upload is True

    def test_dict_defaults_to_uploading(self, in_tmp_cwd, patched_ray):
        """Users must explicitly opt out of exporting results."""
        _, _, upload = _resolve_profiler({"pytorch": {}}, {}, 100, 8, 1, 0)
        assert upload is True

    def test_upload_can_be_disabled(self, in_tmp_cwd, patched_ray):
        """``upload_profiler_results: False`` turns the export off."""
        _, _, upload = _resolve_profiler(
            {"pytorch": {}, "upload_profiler_results": False}, {}, 100, 8, 1, 0
        )
        assert upload is False

    def test_steps_per_epoch_is_computed_from_the_run_shape(
        self, in_tmp_cwd, patched_ray
    ):
        """Row count, batch size, world size, and trainer kwargs all feed in."""
        with patch(f"{_UTIL_MODULE}._build_profiler", return_value=(None, "")) as build:
            _resolve_profiler(
                {"pytorch": {}},
                {"limit_train_batches": 0.5},
                dataset_num_rows=800,
                batch_size=10,
                world_sz=4,
                rank=0,
            )
        # ceil(800 / 4 / 10) = 20, halved by limit_train_batches -> 10.
        build.assert_called_once_with({"pytorch": {}}, 10)

    def test_unknown_row_count_passes_none_through(self, in_tmp_cwd, patched_ray):
        """An unknown row count leaves the schedule to its unbounded default."""
        with patch(f"{_UTIL_MODULE}._build_profiler", return_value=(None, "")) as build:
            _resolve_profiler({"pytorch": {}}, {}, None, 8, 1, 0)
        build.assert_called_once_with({"pytorch": {}}, None)


# -----------------------------------------------------------------------------
# _maybe_export_profiler_results
# -----------------------------------------------------------------------------


class TestMaybeExportProfilerResults:
    """Gating and error containment around the post-``fit`` sink call."""

    def test_sink_called_on_local_rank_0(self, patched_ray):
        """The node-local leader hands the profiler and path to the sink."""
        sink = MagicMock(name="sink")
        profiler = MagicMock(name="profiler")
        logger = MagicMock(name="logger")

        _maybe_export_profiler_results(profiler, "/logs", logger, True, sink)

        sink.assert_called_once_with(profiler, "/logs", logger)

    def test_not_called_on_other_local_ranks(self, patched_ray):
        """Non-leader workers on a node skip the export."""
        patched_ray.get_local_rank.return_value = 1
        sink = MagicMock(name="sink")

        _maybe_export_profiler_results(MagicMock(), "/logs", None, True, sink)

        sink.assert_not_called()

    def test_not_called_without_profiler(self, patched_ray):
        """No profiler means nothing to export."""
        sink = MagicMock(name="sink")
        _maybe_export_profiler_results(None, "", None, True, sink)
        sink.assert_not_called()

    def test_not_called_when_upload_disabled(self, patched_ray):
        """``upload_profiler_results: False`` suppresses the sink."""
        sink = MagicMock(name="sink")
        _maybe_export_profiler_results(MagicMock(), "/logs", None, False, sink)
        sink.assert_not_called()

    def test_no_sink_configured_is_noop(self, patched_ray):
        """A profiler without a sink simply leaves results on disk."""
        # No exception is the assertion here.
        _maybe_export_profiler_results(MagicMock(), "/logs", None, True, None)

    def test_sink_exception_is_swallowed(self, patched_ray):
        """A raising sink never fails a completed training run."""
        sink = MagicMock(name="sink", side_effect=RuntimeError("boom"))
        _maybe_export_profiler_results(MagicMock(), "/logs", None, True, sink)
        sink.assert_called_once()


# -----------------------------------------------------------------------------
# comet_profiler_sink
# -----------------------------------------------------------------------------


class TestCometProfilerSink:
    """Comet upload behavior for each profiler flavor."""

    def test_pytorch_traces_upload_as_tensorboard_folder(self):
        """``PyTorchProfiler`` traces go up as a TensorBoard folder."""
        logger = MagicMock(name="logger")
        profiler = MagicMock(spec=PyTorchProfiler)

        comet_profiler_sink(profiler, "/logs", logger)

        logger.experiment.log_tensorflow_folder.assert_called_once_with("/logs")

    def test_pytorch_upload_failure_is_swallowed(self):
        """A failed TensorBoard upload is logged, not raised."""
        logger = MagicMock(name="logger")
        logger.experiment.log_tensorflow_folder.side_effect = RuntimeError("nope")

        comet_profiler_sink(MagicMock(spec=PyTorchProfiler), "/logs", logger)

    @pytest.mark.parametrize("spec", [SimpleProfiler, AdvancedProfiler])
    def test_text_summaries_upload_as_assets(self, spec, tmp_path):
        """Text-summary profilers upload each ``.txt`` file individually."""
        (tmp_path / "a.txt").write_text("a")
        (tmp_path / "b.txt").write_text("b")
        (tmp_path / "ignored.json").write_text("{}")
        logger = MagicMock(name="logger")

        comet_profiler_sink(MagicMock(spec=spec), str(tmp_path), logger)

        uploaded = {
            os.path.basename(call.args[0])
            for call in logger.experiment.log_asset.call_args_list
        }
        assert uploaded == {"a.txt", "b.txt"}

    def test_no_text_files_is_noop(self, tmp_path):
        """An empty logs directory uploads nothing."""
        logger = MagicMock(name="logger")
        comet_profiler_sink(MagicMock(spec=SimpleProfiler), str(tmp_path), logger)
        logger.experiment.log_asset.assert_not_called()

    def test_asset_upload_failure_does_not_stop_remaining_files(self, tmp_path):
        """One failed asset upload does not abort the rest."""
        (tmp_path / "a.txt").write_text("a")
        (tmp_path / "b.txt").write_text("b")
        logger = MagicMock(name="logger")
        logger.experiment.log_asset.side_effect = [RuntimeError("nope"), None]

        comet_profiler_sink(MagicMock(spec=SimpleProfiler), str(tmp_path), logger)

        assert logger.experiment.log_asset.call_count == 2

    def test_unsupported_profiler_is_skipped(self):
        """XLA and custom profilers have no supported upload format."""
        logger = MagicMock(name="logger")

        comet_profiler_sink(MagicMock(spec=XLAProfiler), "/logs", logger)

        logger.experiment.log_asset.assert_not_called()
        logger.experiment.log_tensorflow_folder.assert_not_called()

    def test_logger_without_experiment_is_skipped(self):
        """A non-Comet logger is skipped rather than crashing the run."""
        # object() has no `experiment` attribute; no exception is the assertion.
        comet_profiler_sink(MagicMock(spec=PyTorchProfiler), "/logs", object())


# -----------------------------------------------------------------------------
# _profiler_output
# -----------------------------------------------------------------------------


class TestProfilerOutput:
    """Classification shared by both built-in sinks."""

    def test_pytorch_profiler_classified_as_dir(self):
        """``PyTorchProfiler`` output is classified as a single directory."""
        result = _profiler_output(MagicMock(spec=PyTorchProfiler), "/logs")
        assert result == ("dir", ["/logs"])

    @pytest.mark.parametrize("spec", [SimpleProfiler, AdvancedProfiler])
    def test_text_profiler_classified_as_files(self, spec, tmp_path):
        """Text-summary profilers are classified as their ``.txt`` files."""
        (tmp_path / "a.txt").write_text("a")
        (tmp_path / "b.txt").write_text("b")
        (tmp_path / "ignored.json").write_text("{}")

        kind, paths = _profiler_output(MagicMock(spec=spec), str(tmp_path))

        assert kind == "files"
        assert {os.path.basename(p) for p in paths} == {"a.txt", "b.txt"}

    def test_no_text_files_returns_none(self, tmp_path):
        """An empty logs directory has nothing to classify."""
        assert _profiler_output(MagicMock(spec=SimpleProfiler), str(tmp_path)) is None

    def test_unsupported_profiler_returns_none(self):
        """XLA and custom profilers have no supported export format."""
        assert _profiler_output(MagicMock(spec=XLAProfiler), "/logs") is None


# -----------------------------------------------------------------------------
# mlflow_profiler_sink
# -----------------------------------------------------------------------------


class TestMlflowProfilerSink:
    """MLflow upload behavior for each profiler flavor."""

    def _logger(self):
        logger = MagicMock(name="logger")
        logger.run_id = "run-123"
        return logger

    def test_pytorch_traces_upload_as_directory(self):
        """``PyTorchProfiler`` traces go up as a directory via ``log_artifacts``."""
        logger = self._logger()
        profiler = MagicMock(spec=PyTorchProfiler)

        mlflow_profiler_sink(profiler, "/logs", logger)

        logger.experiment.log_artifacts.assert_called_once_with(
            "run-123", "/logs", artifact_path="profiler"
        )
        logger.experiment.log_artifact.assert_not_called()

    def test_pytorch_upload_failure_is_swallowed(self):
        """A failed directory upload is logged, not raised."""
        logger = self._logger()
        logger.experiment.log_artifacts.side_effect = RuntimeError("nope")

        mlflow_profiler_sink(MagicMock(spec=PyTorchProfiler), "/logs", logger)

    @pytest.mark.parametrize("spec", [SimpleProfiler, AdvancedProfiler])
    def test_text_summaries_upload_per_file(self, spec, tmp_path):
        """Text-summary profilers upload each ``.txt`` file individually."""
        (tmp_path / "a.txt").write_text("a")
        (tmp_path / "b.txt").write_text("b")
        (tmp_path / "ignored.json").write_text("{}")
        logger = self._logger()

        mlflow_profiler_sink(MagicMock(spec=spec), str(tmp_path), logger)

        uploaded = {
            os.path.basename(call.args[1])
            for call in logger.experiment.log_artifact.call_args_list
        }
        assert uploaded == {"a.txt", "b.txt"}
        for call in logger.experiment.log_artifact.call_args_list:
            assert call.args[0] == "run-123"
            assert call.kwargs == {"artifact_path": "profiler"}
        logger.experiment.log_artifacts.assert_not_called()

    def test_no_text_files_is_noop(self, tmp_path):
        """An empty logs directory uploads nothing."""
        logger = self._logger()
        mlflow_profiler_sink(MagicMock(spec=SimpleProfiler), str(tmp_path), logger)
        logger.experiment.log_artifact.assert_not_called()

    def test_asset_upload_failure_does_not_stop_remaining_files(self, tmp_path):
        """One failed asset upload does not abort the rest."""
        (tmp_path / "a.txt").write_text("a")
        (tmp_path / "b.txt").write_text("b")
        logger = self._logger()
        logger.experiment.log_artifact.side_effect = [RuntimeError("nope"), None]

        mlflow_profiler_sink(MagicMock(spec=SimpleProfiler), str(tmp_path), logger)

        assert logger.experiment.log_artifact.call_count == 2

    def test_unsupported_profiler_is_skipped(self):
        """XLA and custom profilers have no supported upload format."""
        logger = self._logger()

        mlflow_profiler_sink(MagicMock(spec=XLAProfiler), "/logs", logger)

        logger.experiment.log_artifact.assert_not_called()
        logger.experiment.log_artifacts.assert_not_called()

    def test_logger_without_experiment_is_skipped(self):
        """A logger with no MLflow client is skipped rather than crashing the run."""
        # object() has no `experiment`/`run_id` attribute; no exception is the assertion.
        mlflow_profiler_sink(MagicMock(spec=PyTorchProfiler), "/logs", object())

    def test_logger_without_run_id_is_skipped(self):
        """A logger with a client but no run id is skipped."""
        logger = MagicMock(name="logger")
        logger.run_id = None

        mlflow_profiler_sink(MagicMock(spec=PyTorchProfiler), "/logs", logger)

        logger.experiment.log_artifacts.assert_not_called()
