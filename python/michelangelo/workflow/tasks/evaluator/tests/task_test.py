"""Tests for michelangelo.workflow.tasks.evaluator.task."""

from __future__ import annotations

from unittest import TestCase
from unittest.mock import MagicMock, patch

import pytest

ray = pytest.importorskip("ray")
pytest.importorskip("torch")
pytest.importorskip("torchmetrics")

import pandas as pd  # noqa: E402

from michelangelo.lib.evaluator.report_generator import (  # noqa: E402
    build_combined_report,
)
from michelangelo.lib.exceptions import (  # noqa: E402
    ConfigurationError as LibConfigurationError,
)
from michelangelo.workflow.schema.evaluator import (  # noqa: E402
    ColumnMapping,
    EvaluatorConfig,
    MetricConfig,
    SanityCheck,
    SanityCheckOperator,
    Step,
)
from michelangelo.workflow.schema.exceptions import ConfigurationError  # noqa: E402
from michelangelo.workflow.tasks.evaluator.exceptions import (  # noqa: E402
    SanityCheckFailedError,
)
from michelangelo.workflow.tasks.evaluator.preprocessor import (  # noqa: E402
    declare_inputs,
)
from michelangelo.workflow.tasks.evaluator.task import (  # noqa: E402
    COMBINED_REPORT_KEY,
    _apply_preprocessor,
    _evaluate_segment,
    _fill_segment_nulls,
    _load_dataset,
    _resolve_preprocessor,
    evaluate,
)

_TASK = "michelangelo.workflow.tasks.evaluator.task"

# Ray Data's groupby shuffles before it maps, so a single-CPU cluster stalls the
# segmented paths. Four is enough to keep the shuffle and the map concurrent.
_RAY_TEST_CPUS = 4


def _init_ray() -> None:
    """Start (or reuse) a small local Ray instance for the dataset-backed tests."""
    ray.init(ignore_reinit_error=True, num_cpus=_RAY_TEST_CPUS)


class _FakeDataset:
    """Stand-in for a DatasetVariable backed by an in-memory Ray dataset."""

    def __init__(self, df: pd.DataFrame):
        """Hold the rows this dataset yields when loaded."""
        self._df = df
        self.value = None
        self.load_columns = None

    def load_ray_dataset(self, columns=None, **_kwargs):
        """Materialize the rows, projected to ``columns`` when given."""
        self.load_columns = list(columns) if columns else None
        df = self._df
        if columns:
            present = [c for c in columns if c in df.columns]
            df = df[present]
        self.value = ray.data.from_pandas(df) if len(df) else ray.data.from_items([])

    def __bool__(self) -> bool:
        """Mirror DatasetVariable's truthiness so ``if not dataset`` works."""
        return True


def _frame(n: int = 40, *, segment: bool = False) -> pd.DataFrame:
    """Build a small, perfectly separable binary classification frame."""
    labels = [i % 2 for i in range(n)]
    data = {
        "score": [0.9 if label else 0.1 for label in labels],
        "label": labels,
        "unused_feature": list(range(n)),
    }
    if segment:
        data["region"] = ["us" if i < n // 2 else "eu" for i in range(n)]
    return pd.DataFrame(data)


def _config(**overrides) -> EvaluatorConfig:
    """Build an EvaluatorConfig computing a single binary AUROC."""
    params = {
        "metrics": [
            MetricConfig(
                name="auc",
                metric="torchmetrics.classification.BinaryAUROC",
                columns=ColumnMapping(prediction_col="score", target_col="label"),
            )
        ]
    }
    params.update(overrides)
    return EvaluatorConfig(**params)


@declare_inputs(["raw_score", "label"])
def _rename_score(df: pd.DataFrame) -> pd.DataFrame:
    """Test preprocessor: expose ``raw_score`` under the name the metric reads."""
    df = df.copy()
    df["score"] = df["raw_score"]
    return df


@declare_inputs(["raw_score"])
def _drops_label(df: pd.DataFrame) -> pd.DataFrame:
    """Test preprocessor that fails to produce the metric's target column."""
    return df.copy()


_NOT_CALLABLE = "a string, not a function"


def _undecorated(df: pd.DataFrame) -> pd.DataFrame:
    """Test preprocessor missing the declare_inputs decoration."""
    return df


class EvaluateGuardsTest(TestCase):
    """Tests for the configuration guards in evaluate()."""

    def test_no_metrics_raises(self):
        """A config declaring nothing to compute is rejected up front."""
        with pytest.raises(ConfigurationError, match="declares no metrics"):
            evaluate(EvaluatorConfig(), {})

    def test_step_based_config_raises(self):
        """The legacy step-based path is not supported by this evaluator."""
        config = EvaluatorConfig(steps=[Step(processor="pkg.mod.fn", name="s1")])
        with pytest.raises(ConfigurationError, match="Step-based evaluation"):
            evaluate(config, {})

    def test_step_config_is_rejected_before_metrics(self):
        """A config with both steps and metrics still names steps as the problem."""
        config = _config(steps=[Step(processor="pkg.mod.fn", name="s1")])
        with pytest.raises(ConfigurationError, match="Step-based evaluation"):
            evaluate(config, {})


class ResolvePreprocessorTest(TestCase):
    """Tests for _resolve_preprocessor."""

    def test_returns_none_when_unset(self):
        """No preprocessor configured resolves to None."""
        assert _resolve_preprocessor(_config()) is None

    def test_resolves_decorated_callable(self):
        """A decorated function resolves and keeps its declared columns."""
        path = f"{__name__}._rename_score"
        resolved = _resolve_preprocessor(_config(data_preprocessor=path))
        assert resolved.input_columns == ["raw_score", "label"]

    def test_unimportable_path_raises(self):
        """A path naming a module that does not exist is a config error."""
        config = _config(data_preprocessor="no.such.module.fn")
        with pytest.raises(ConfigurationError, match="could not be imported"):
            _resolve_preprocessor(config)

    def test_non_callable_raises(self):
        """A path resolving to a non-callable is a config error."""
        config = _config(data_preprocessor=f"{__name__}._NOT_CALLABLE")
        with pytest.raises(ConfigurationError, match="non-callable"):
            _resolve_preprocessor(config)

    def test_missing_input_columns_raises(self):
        """An undecorated preprocessor cannot drive column projection."""
        config = _config(data_preprocessor=f"{__name__}._undecorated")
        with pytest.raises(ConfigurationError, match="input_columns"):
            _resolve_preprocessor(config)


class SegmentHelpersTest(TestCase):
    """Tests for the segment-level helper functions."""

    def test_fill_segment_nulls_replaces_nulls(self):
        """Nulls become their own explicit UNKNOWN segment."""
        batch = pd.DataFrame({"region": ["us", None], "score": [0.1, 0.2]})
        filled = _fill_segment_nulls(batch, ["region"])
        assert list(filled["region"]) == ["us", "UNKNOWN"]

    def test_fill_segment_nulls_ignores_absent_column(self):
        """A segment column missing from the batch is skipped, not an error."""
        batch = pd.DataFrame({"score": [0.1]})
        assert list(_fill_segment_nulls(batch, ["region"]).columns) == ["score"]

    def test_evaluate_segment_returns_single_row(self):
        """One segment collapses to one row of metrics plus its identity."""
        evaluator = MagicMock()
        evaluator.evaluate.return_value = {"auc": 0.75}
        group = pd.DataFrame({"region": ["us", "us"], "score": [0.1, 0.9]})

        row = _evaluate_segment(group, evaluator, ["region"])

        assert len(row) == 1
        assert row["auc"].iloc[0] == 0.75
        assert row["region"].iloc[0] == "us"
        assert row["_segment_row_count"].iloc[0] == 2


class LoadDatasetTest(TestCase):
    """Tests for _load_dataset."""

    @classmethod
    def setUpClass(cls):
        """Start a small local Ray instance for the dataset helpers."""
        _init_ray()

    def test_projects_requested_columns(self):
        """Only the requested columns are read from the source."""
        dataset = _FakeDataset(_frame())
        loaded = _load_dataset(
            dataset, "train", {"score", "label"}, has_preprocessor=False
        )
        assert set(loaded.schema().names) == {"score", "label"}
        assert "unused_feature" not in loaded.schema().names

    def test_missing_column_raises(self):
        """A column the metrics need but the dataset lacks fails the read."""
        dataset = _FakeDataset(_frame())
        with pytest.raises(ValueError, match="required by the configured metrics"):
            _load_dataset(dataset, "train", {"score", "absent"}, has_preprocessor=False)

    def test_missing_column_blames_preprocessor_when_present(self):
        """With a preprocessor, the message points at its declared inputs."""
        dataset = _FakeDataset(_frame())
        with pytest.raises(ValueError, match="data_preprocessor"):
            _load_dataset(dataset, "train", {"score", "absent"}, has_preprocessor=True)

    def test_empty_test_dataset_is_skipped(self):
        """An absent test split is optional, not an error."""
        dataset = _FakeDataset(pd.DataFrame({"score": [], "label": []}))
        assert _load_dataset(dataset, "test", {"score"}, has_preprocessor=False) is None

    def test_empty_train_dataset_raises(self):
        """An empty non-test dataset means an upstream task produced nothing."""
        dataset = _FakeDataset(pd.DataFrame({"score": [], "label": []}))
        with pytest.raises(ValueError, match="None/empty"):
            _load_dataset(dataset, "train", {"score"}, has_preprocessor=False)


class ApplyPreprocessorTest(TestCase):
    """Tests for _apply_preprocessor."""

    @classmethod
    def setUpClass(cls):
        """Start a small local Ray instance."""
        _init_ray()

    def test_transform_is_applied(self):
        """The preprocessor's output columns reach the evaluator."""
        source = _frame().rename(columns={"score": "raw_score"})
        ray_dataset = ray.data.from_pandas(source[["raw_score", "label"]])

        result = _apply_preprocessor(
            ray_dataset, _rename_score, "train", {"score", "label"}
        )

        assert "score" in result.schema().names

    def test_missing_metric_column_after_transform_raises(self):
        """A transform that drops a metric column fails before evaluation."""
        source = _frame().rename(columns={"score": "raw_score"})
        ray_dataset = ray.data.from_pandas(source[["raw_score"]])

        with pytest.raises(ValueError, match="missing columns required by metrics"):
            _apply_preprocessor(ray_dataset, _drops_label, "train", {"score", "label"})


class EvaluateTest(TestCase):
    """End-to-end tests for evaluate() over in-memory datasets."""

    @classmethod
    def setUpClass(cls):
        """Start a small local Ray instance."""
        _init_ray()

    def setUp(self):
        """Stub out summary persistence so no dataset is written to disk."""
        patcher = patch(f"{_TASK}._save_summary")
        self.save_summary = patcher.start()
        self.addCleanup(patcher.stop)
        self.saved: list[pd.DataFrame] = []
        self.save_summary.side_effect = lambda df: self.saved.append(df) or MagicMock()

    def test_evaluates_a_single_dataset(self):
        """A dataset with no segmentation yields one aggregate row."""
        result = evaluate(_config(), {"test": _FakeDataset(_frame())})

        assert list(result.metrics) == ["test"]
        assert len(self.saved) == 1
        assert self.saved[0]["auc"].iloc[0] == pytest.approx(1.0)

    def test_produces_per_dataset_and_combined_reports(self):
        """Each dataset gets its own report alongside the combined one."""
        datasets = {
            "validation": _FakeDataset(_frame()),
            "test": _FakeDataset(_frame()),
        }

        result = evaluate(_config(), datasets)

        assert set(result.reports) == {"validation", "test", COMBINED_REPORT_KEY}
        assert result.reports[COMBINED_REPORT_KEY].value.metadata.name.startswith(
            "evaluation-report"
        )

    def test_skips_none_datasets(self):
        """A None dataset is skipped rather than failing the run."""
        result = evaluate(_config(), {"train": None, "test": _FakeDataset(_frame())})

        assert list(result.metrics) == ["test"]

    def test_reads_only_the_columns_the_metrics_need(self):
        """Column projection is pushed into the read, not applied afterwards."""
        dataset = _FakeDataset(_frame())

        evaluate(_config(), {"test": dataset})

        assert set(dataset.load_columns) == {"score", "label"}

    def test_preprocessor_drives_column_projection(self):
        """With a preprocessor, its declared inputs drive the read instead."""
        source = _frame().rename(columns={"score": "raw_score"})
        dataset = _FakeDataset(source)
        config = _config(data_preprocessor=f"{__name__}._rename_score")

        result = evaluate(config, {"test": dataset})

        assert set(dataset.load_columns) == {"raw_score", "label"}
        assert self.saved[0]["auc"].iloc[0] == pytest.approx(1.0)
        assert list(result.metrics) == ["test"]

    def test_segmented_evaluation_adds_a_row_per_segment(self):
        """Segmentation yields the aggregate row plus one row per segment."""
        config = _config(segment_columns=["region"])

        evaluate(config, {"test": _FakeDataset(_frame(segment=True))})

        summary = self.saved[0]
        assert len(summary) == 3
        assert set(summary["region"]) == {"GLOBAL", "us", "eu"}

    def test_global_row_can_be_disabled(self):
        """enable_global_segment_evaluation=False leaves only segment rows."""
        config = _config(
            segment_columns=["region"], enable_global_segment_evaluation=False
        )

        evaluate(config, {"test": _FakeDataset(_frame(segment=True))})

        assert set(self.saved[0]["region"]) == {"us", "eu"}

    def test_thin_segments_are_dropped(self):
        """A segment below min_segment_row_count never reaches the report."""
        frame = _frame(segment=True)
        # One lone 'ap' row: too thin to conclude anything from.
        frame.loc[frame.index[0], "region"] = "ap"
        config = _config(segment_columns=["region"], min_segment_row_count=5)

        evaluate(config, {"test": _FakeDataset(frame)})

        assert "ap" not in set(self.saved[0]["region"])


class SanityCheckGatingTest(TestCase):
    """Tests for sanity-check gating in evaluate()."""

    @classmethod
    def setUpClass(cls):
        """Start a small local Ray instance."""
        _init_ray()

    def setUp(self):
        """Stub out summary persistence so no dataset is written to disk."""
        patcher = patch(f"{_TASK}._save_summary")
        patcher.start()
        self.addCleanup(patcher.stop)

    def test_passing_check_completes(self):
        """A metric above its threshold lets the run finish."""
        config = _config(
            sanity_checks=[SanityCheck(metric="auc", threshold=0.5, datasets=["test"])]
        )

        result = evaluate(config, {"test": _FakeDataset(_frame())})

        assert list(result.metrics) == ["test"]

    def test_breaching_check_raises(self):
        """A metric below its threshold fails the run."""
        config = _config(
            sanity_checks=[SanityCheck(metric="auc", threshold=1.5, datasets=["test"])]
        )

        with pytest.raises(SanityCheckFailedError, match="auc"):
            evaluate(config, {"test": _FakeDataset(_frame())})

    def test_breaching_lte_check_raises(self):
        """The comparison operator is honoured, not assumed to be >=."""
        config = _config(
            sanity_checks=[
                SanityCheck(
                    metric="auc",
                    threshold=0.5,
                    operator=SanityCheckOperator.LTE,
                    datasets=["test"],
                )
            ]
        )

        with pytest.raises(SanityCheckFailedError):
            evaluate(config, {"test": _FakeDataset(_frame())})

    def test_checks_on_an_unevaluated_dataset_raise(self):
        """A check naming a dataset that was never evaluated is not a pass."""
        config = _config(
            sanity_checks=[SanityCheck(metric="auc", threshold=0.5, datasets=["train"])]
        )

        with pytest.raises(ConfigurationError):
            evaluate(config, {"test": _FakeDataset(_frame())})

    def test_library_config_errors_are_translated(self):
        """The library's own ConfigurationError does not leak to the caller."""
        config = _config(
            sanity_checks=[SanityCheck(metric="auc", threshold=0.5, datasets=["train"])]
        )

        with pytest.raises(ConfigurationError) as caught:
            evaluate(config, {"test": _FakeDataset(_frame())})

        # Translated, not re-wrapped blindly: the library's message survives.
        assert "train" in str(caught.value)
        assert isinstance(caught.value.__cause__, LibConfigurationError)

    def test_no_gateable_scope_raises(self):
        """Gating nothing at all is a configuration error, not a pass."""
        config = _config(
            segment_columns=["region"],
            enable_global_segment_evaluation=False,
            # Every segment is thinner than this, so none survives to be gated.
            min_segment_row_count=1000,
            sanity_checks=[SanityCheck(metric="auc", threshold=0.5, datasets=["test"])],
        )

        with pytest.raises(ConfigurationError, match="no scope"):
            evaluate(config, {"test": _FakeDataset(_frame(segment=True))})


class ReportsBeforeGatingTest(TestCase):
    """The reports an operator needs are written before the run is failed."""

    @classmethod
    def setUpClass(cls):
        """Start a small local Ray instance."""
        _init_ray()

    @patch(f"{_TASK}._save_summary")
    def test_reports_are_written_before_the_failure(self, _mock_save):
        """A breach does not cost the operator the report that explains it."""
        config = _config(
            sanity_checks=[SanityCheck(metric="auc", threshold=1.5, datasets=["test"])]
        )

        with (
            patch(f"{_TASK}.build_combined_report", wraps=build_combined_report) as spy,
            pytest.raises(SanityCheckFailedError),
        ):
            evaluate(config, {"test": _FakeDataset(_frame())})
        spy.assert_called_once()
