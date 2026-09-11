"""Tests for the MLflow-backed ``MlflowExperimentStore``.

Covers ``michelangelo.lib.trainer.torch.pytorch_lightning.experiment_store_mlflow``:
the marker-run round-trip against a fully faked ``MlflowClient`` (no live
server), the "nothing to resume" paths (missing experiment, no matching run,
server errors — all returning ``None`` without raising), the best-effort
swallow on ``track`` failures, filter-string escaping plus the client-side
identity re-check, and structural conformance to the :class:`ExperimentStore`
protocol.
"""

from __future__ import annotations

import sys
from types import ModuleType, SimpleNamespace
from unittest.mock import MagicMock

import pytest

# Importing the store pulls in the package ``__init__`` which eagerly imports the
# Ray/Lightning-backed trainer. Skip cleanly if those heavy deps are missing.
# mlflow itself is NOT required: the store imports it lazily inside ``_client``
# and these tests substitute a fake ``mlflow.tracking`` module, so they run in
# environments (like CI) where the ``trainer-mlflow`` extra is not installed.
pytest.importorskip("ray")
pytest.importorskip("torch")
pytest.importorskip("pytorch_lightning")

from michelangelo.lib.trainer.torch.pytorch_lightning.experiment_store_mlflow import (
    _TAG_EXPERIMENT_PATH,
    _TAG_RUN_NAME,
    _TAG_STORAGE_PATH,
    MlflowExperimentStore,
)
from michelangelo.lib.trainer.torch.pytorch_lightning.schema import (
    ExperimentStore,
)


class _FakeMlflowClient:
    """In-memory stand-in for ``mlflow.tracking.MlflowClient``.

    Implements just the five calls the store makes. ``search_runs`` ignores
    the filter (it is recorded for assertion instead) and returns all runs
    newest-first, mirroring the store's ``order_by`` — the store's client-side
    identity re-check is what narrows to the right run, which is exactly the
    behavior under test.
    """

    def __init__(self, tracking_uri=None):
        self.tracking_uri = tracking_uri
        self.experiments: dict[str, str] = {}  # name -> experiment_id
        self.runs: list[SimpleNamespace] = []  # newest appended last
        self.terminated: list[tuple[str, str]] = []
        self.last_search_kwargs: dict | None = None
        self._counter = 0

    def get_experiment_by_name(self, name):
        if name not in self.experiments:
            return None
        return SimpleNamespace(experiment_id=self.experiments[name], name=name)

    def create_experiment(self, name):
        experiment_id = f"exp-{len(self.experiments)}"
        self.experiments[name] = experiment_id
        return experiment_id

    def create_run(self, experiment_id, tags=None, run_name=None):
        self._counter += 1
        run = SimpleNamespace(
            info=SimpleNamespace(
                run_id=f"run-{self._counter}", experiment_id=experiment_id
            ),
            data=SimpleNamespace(tags=dict(tags or {})),
        )
        self.runs.append(run)
        return run

    def set_terminated(self, run_id, status):
        self.terminated.append((run_id, status))

    def search_runs(self, experiment_ids, filter_string, order_by, max_results):
        self.last_search_kwargs = {
            "experiment_ids": experiment_ids,
            "filter_string": filter_string,
            "order_by": order_by,
            "max_results": max_results,
        }
        return list(reversed(self.runs))[:max_results]


def _install_mlflow_stub(monkeypatch, constructor):
    """Plant stub ``mlflow``/``mlflow.tracking`` modules exposing ``constructor``.

    The store imports mlflow lazily (``from mlflow.tracking import
    MlflowClient`` inside ``_client``), so seeding ``sys.modules`` makes that
    import resolve to the stub whether or not real mlflow is installed —
    letting these tests run in environments without the ``trainer-mlflow``
    extra (like CI).
    """
    tracking = ModuleType("mlflow.tracking")
    tracking.MlflowClient = constructor
    mlflow_stub = ModuleType("mlflow")
    mlflow_stub.tracking = tracking
    monkeypatch.setitem(sys.modules, "mlflow", mlflow_stub)
    monkeypatch.setitem(sys.modules, "mlflow.tracking", tracking)


@pytest.fixture()
def fake_client(monkeypatch):
    """A fresh fake client, installed as the ``MlflowClient`` constructor."""
    client = _FakeMlflowClient()
    _install_mlflow_stub(monkeypatch, MagicMock(return_value=client))
    yield client


class TestProtocolConformance:
    """``MlflowExperimentStore`` structurally satisfies the protocol."""

    def test_is_experiment_store(self):
        """The store passes the ``@runtime_checkable`` protocol check."""
        assert isinstance(MlflowExperimentStore(), ExperimentStore)


class TestRoundTrip:
    """Track-then-locate round-trip against the fake client."""

    def test_track_then_locate_returns_experiment_path(self, fake_client):
        """``track`` creates a tagged marker run that ``locate_resumable`` finds."""
        store = MlflowExperimentStore()
        store.track(
            storage_path="/root/runs",
            run_name="run1",
            experiment_path="/exp/run1_dir",
        )

        # One marker run, tagged with the full identity, and terminated.
        assert len(fake_client.runs) == 1
        tags = fake_client.runs[0].data.tags
        assert tags[_TAG_RUN_NAME] == "run1"
        assert tags[_TAG_STORAGE_PATH] == "/root/runs"
        assert tags[_TAG_EXPERIMENT_PATH] == "/exp/run1_dir"
        assert fake_client.terminated == [(fake_client.runs[0].info.run_id, "FINISHED")]

        located = store.locate_resumable(storage_path="/root/runs", run_name="run1")
        assert located == "/exp/run1_dir"

    def test_most_recent_marker_wins(self, fake_client):
        """A second ``track`` for the same identity shadows the first."""
        store = MlflowExperimentStore()
        store.track(
            storage_path="/root/runs", run_name="run1", experiment_path="/exp/old"
        )
        store.track(
            storage_path="/root/runs", run_name="run1", experiment_path="/exp/new"
        )
        assert len(fake_client.runs) == 2  # accumulates, does not overwrite
        assert (
            store.locate_resumable(storage_path="/root/runs", run_name="run1")
            == "/exp/new"
        )

    def test_distinct_identities_are_isolated(self, fake_client):
        """The client-side re-check keys strictly on ``(storage_path, run_name)``."""
        store = MlflowExperimentStore()
        store.track(storage_path="/root", run_name="a", experiment_path="/exp/a")
        store.track(storage_path="/root", run_name="b", experiment_path="/exp/b")
        store.track(storage_path="/other", run_name="a", experiment_path="/exp/oa")
        assert store.locate_resumable(storage_path="/root", run_name="a") == "/exp/a"
        assert store.locate_resumable(storage_path="/root", run_name="b") == "/exp/b"
        assert store.locate_resumable(storage_path="/other", run_name="a") == "/exp/oa"

    def test_experiment_created_once_and_reused(self, fake_client):
        """The marker experiment is created on first track, then reused."""
        store = MlflowExperimentStore(experiment_name="my-resume")
        store.track(storage_path="/root", run_name="r", experiment_path="/e1")
        store.track(storage_path="/root", run_name="r", experiment_path="/e2")
        assert list(fake_client.experiments) == ["my-resume"]


class TestSearchQuery:
    """The search request carries the identity filter and ordering."""

    def test_filter_and_order(self, fake_client):
        """Both identity tags are filtered; newest-first, small window."""
        store = MlflowExperimentStore()
        store.track(storage_path="/root", run_name="run1", experiment_path="/exp")
        store.locate_resumable(storage_path="/root", run_name="run1")
        kwargs = fake_client.last_search_kwargs
        assert f"tags.{_TAG_RUN_NAME} = 'run1'" in kwargs["filter_string"]
        assert f"tags.{_TAG_STORAGE_PATH} = '/root'" in kwargs["filter_string"]
        assert kwargs["order_by"] == ["attributes.start_time DESC"]
        assert kwargs["max_results"] >= 1

    def test_single_quote_switches_to_double_quoted_literal(self, fake_client):
        """MLflow filters cannot escape quotes; the literal's quote style flips."""
        store = MlflowExperimentStore()
        store.track(storage_path="/root", run_name="it's run", experiment_path="/exp/q")
        assert (
            store.locate_resumable(storage_path="/root", run_name="it's run")
            == "/exp/q"
        )
        assert '"it\'s run"' in fake_client.last_search_kwargs["filter_string"]

    def test_double_quote_keeps_single_quoted_literal(self, fake_client):
        """A double quote in the identity stays inside a single-quoted literal."""
        store = MlflowExperimentStore()
        store.track(
            storage_path="/root", run_name='say "hi"', experiment_path="/exp/dq"
        )
        assert (
            store.locate_resumable(storage_path="/root", run_name='say "hi"')
            == "/exp/dq"
        )
        assert "'say \"hi\"'" in fake_client.last_search_kwargs["filter_string"]

    def test_both_quote_styles_degrade_to_none_without_search(self, fake_client):
        """An identity with both quote styles is unfilterable -> nothing to resume."""
        store = MlflowExperimentStore()
        tricky = 'it\'s "tricky"'
        store.track(storage_path="/root", run_name=tricky, experiment_path="/exp/t")
        assert store.locate_resumable(storage_path="/root", run_name=tricky) is None
        # Short-circuited before issuing a search.
        assert fake_client.last_search_kwargs is None

    def test_identity_recheck_skips_mismatched_runs(self, fake_client):
        """A run the server returns despite a different identity is skipped."""
        store = MlflowExperimentStore()
        store.track(storage_path="/root", run_name="other", experiment_path="/exp/x")
        # The fake search returns every run regardless of filter; only an
        # exact client-side identity match may be resumed.
        assert store.locate_resumable(storage_path="/root", run_name="run1") is None


class TestNothingToResume:
    """``locate_resumable`` returns ``None`` without raising on the sad paths."""

    def test_missing_experiment_returns_none(self, fake_client):
        """No marker experiment yet → nothing to resume."""
        store = MlflowExperimentStore()
        assert store.locate_resumable(storage_path="/root", run_name="run1") is None

    def test_no_matching_runs_returns_none(self, fake_client):
        """Experiment exists but holds no marker for this identity."""
        store = MlflowExperimentStore()
        fake_client.create_experiment("michelangelo-resume")
        assert store.locate_resumable(storage_path="/root", run_name="run1") is None

    def test_marker_without_experiment_path_returns_none(self, fake_client):
        """A marker run missing the payload tag → ``None``."""
        store = MlflowExperimentStore()
        experiment_id = fake_client.create_experiment("michelangelo-resume")
        fake_client.create_run(
            experiment_id,
            tags={_TAG_RUN_NAME: "run1", _TAG_STORAGE_PATH: "/root"},
        )
        assert store.locate_resumable(storage_path="/root", run_name="run1") is None

    def test_locate_swallows_client_errors(self, monkeypatch):
        """An unreachable server is swallowed; ``locate`` returns ``None``."""
        store = MlflowExperimentStore()
        raising = MagicMock()
        raising.get_experiment_by_name.side_effect = ConnectionError("boom")
        _install_mlflow_stub(monkeypatch, MagicMock(return_value=raising))
        assert store.locate_resumable(storage_path="/root", run_name="run1") is None

    def test_empty_identity_returns_none_without_client(self, monkeypatch):
        """Empty ``storage_path``/``run_name`` short-circuits before any client use."""
        store = MlflowExperimentStore()
        constructor = MagicMock()
        _install_mlflow_stub(monkeypatch, constructor)
        assert store.locate_resumable(storage_path="", run_name="run1") is None
        assert store.locate_resumable(storage_path="/root", run_name="") is None
        constructor.assert_not_called()


class TestMlflowNotInstalled:
    """The lazy import degrades per the never-raise contract when mlflow is absent."""

    def test_track_and_locate_degrade_gracefully(self, monkeypatch):
        """With mlflow unimportable, ``track`` swallows and ``locate`` returns None."""
        store = MlflowExperimentStore()
        # Making the module entry None causes ``from mlflow.tracking import
        # MlflowClient`` to raise ImportError, simulating a missing extra.
        monkeypatch.setitem(sys.modules, "mlflow.tracking", None)
        store.track(storage_path="/root", run_name="run1", experiment_path="/exp")
        assert store.locate_resumable(storage_path="/root", run_name="run1") is None

    def test_client_import_error_message_names_the_extra(self, monkeypatch):
        """The ImportError tells the user which extra to install."""
        store = MlflowExperimentStore()
        monkeypatch.setitem(sys.modules, "mlflow.tracking", None)
        with pytest.raises(ImportError, match="trainer-mlflow"):
            store._client()


class TestTrackNeverRaises:
    """``track`` is best-effort: a failure must never propagate."""

    def test_track_swallows_lost_race_with_vanishing_experiment(self, monkeypatch):
        """Create fails and the re-read still misses → swallowed, no run created."""
        store = MlflowExperimentStore()
        client = MagicMock()
        client.get_experiment_by_name.return_value = None
        client.create_experiment.side_effect = RuntimeError("server hiccup")
        _install_mlflow_stub(monkeypatch, MagicMock(return_value=client))
        # Must not raise, even though _ensure_experiment re-raises internally.
        store.track(storage_path="/root", run_name="run1", experiment_path="/exp/dir")
        client.create_run.assert_not_called()

    def test_track_swallows_client_errors(self, monkeypatch):
        """An unreachable server during ``track`` does not propagate."""
        store = MlflowExperimentStore()
        raising = MagicMock()
        raising.get_experiment_by_name.side_effect = ConnectionError("boom")
        _install_mlflow_stub(monkeypatch, MagicMock(return_value=raising))
        # Must not raise.
        store.track(storage_path="/root", run_name="run1", experiment_path="/exp/dir")

    def test_track_with_empty_identity_writes_nothing(self, fake_client):
        """Empty ``storage_path``/``run_name`` short-circuits: no run created."""
        store = MlflowExperimentStore()
        store.track(storage_path="", run_name="run1", experiment_path="/exp")
        store.track(storage_path="/root", run_name="", experiment_path="/exp")
        assert fake_client.runs == []

    def test_track_survives_experiment_creation_race(self, monkeypatch):
        """Losing the create-experiment race falls back to the winner's id."""
        store = MlflowExperimentStore()
        client = MagicMock()
        experiment = SimpleNamespace(experiment_id="exp-9")
        # First lookup misses, create fails (someone else won), re-read hits.
        client.get_experiment_by_name.side_effect = [None, experiment]
        client.create_experiment.side_effect = RuntimeError("already exists")
        client.create_run.return_value = SimpleNamespace(
            info=SimpleNamespace(run_id="run-1"),
            data=SimpleNamespace(tags={}),
        )
        _install_mlflow_stub(monkeypatch, MagicMock(return_value=client))
        store.track(storage_path="/root", run_name="run1", experiment_path="/exp/dir")
        client.create_run.assert_called_once()
        assert client.create_run.call_args.args[0] == "exp-9"


class TestConstruction:
    """Constructor arguments flow through; the store stays picklable."""

    def test_tracking_uri_forwarded(self, monkeypatch):
        """The ``tracking_uri`` reaches the ``MlflowClient`` constructor."""
        store = MlflowExperimentStore(tracking_uri="http://mlflow.example.com")
        constructor = MagicMock()
        constructor.return_value.get_experiment_by_name.return_value = None
        _install_mlflow_stub(monkeypatch, constructor)
        store.locate_resumable(storage_path="/root", run_name="run1")
        constructor.assert_called_once_with(tracking_uri="http://mlflow.example.com")

    def test_store_is_picklable(self):
        """The store holds only plain strings, so it survives pickling to workers."""
        import pickle

        restored = pickle.loads(
            pickle.dumps(
                MlflowExperimentStore(
                    tracking_uri="http://mlflow.example.com",
                    experiment_name="team-x",
                )
            )
        )
        assert isinstance(restored, MlflowExperimentStore)
        assert restored._tracking_uri == "http://mlflow.example.com"
        assert restored._experiment_name == "team-x"
