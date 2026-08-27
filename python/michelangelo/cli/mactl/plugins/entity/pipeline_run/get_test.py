"""Unit tests for pipeline_run get filters + display columns."""

from types import SimpleNamespace
from unittest import TestCase
from unittest.mock import MagicMock, patch

from michelangelo.cli.mactl.plugins.entity.pipeline_run.get import (
    _CRITERION_OPERATOR_LIKE,
    _DEFAULT_ENV_LABEL,
    _DEFAULT_STATE_PB2_MODULE,
    _env_label,
    _load_state_pb2,
    _render_env,
    _render_revision,
    _render_state,
    _render_user,
    add_get_filters,
)


def _stub_pb2(names_with_ids):
    """Build a stand-in pipeline_run_pb2 with the given PipelineRunState enum."""
    stub = MagicMock()
    stub.PipelineRunState.DESCRIPTOR.values = [
        SimpleNamespace(name=n, number=i) for n, i in names_with_ids
    ]

    def _name(value):
        for n, i in names_with_ids:
            if i == value:
                return n
        raise ValueError(f"{value} is not a valid PipelineRunState")

    stub.PipelineRunState.Name.side_effect = _name
    return stub


_STATE_OSS = [
    ("PIPELINE_RUN_STATE_INVALID", 0),
    ("PIPELINE_RUN_STATE_PENDING", 1),
    ("PIPELINE_RUN_STATE_RUNNING", 2),
    ("PIPELINE_RUN_STATE_SUCCEEDED", 3),
    ("PIPELINE_RUN_STATE_KILLED", 4),
]


class LoadStatePb2Test(TestCase):
    """The proto module path is configurable via ``pipeline_run_state_pb2_module``."""

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline_run.get.importlib.import_module"
    )
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline_run.get.load_config")
    def test_defaults_to_v2(self, mock_load, mock_import):
        """Missing config key → default OSS v2 module path."""
        mock_load.return_value = {}
        _load_state_pb2()
        mock_import.assert_called_once_with(_DEFAULT_STATE_PB2_MODULE)

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline_run.get.importlib.import_module"
    )
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline_run.get.load_config")
    def test_honors_override(self, mock_load, mock_import):
        """Config override reaches importlib."""
        mock_load.return_value = {
            "pipeline_run_state_pb2_module": "downstream.pipeline_run_pb2"
        }
        _load_state_pb2()
        mock_import.assert_called_once_with("downstream.pipeline_run_pb2")


class EnvLabelTest(TestCase):
    """The env label key is configurable via ``pipeline_run_environment_label``."""

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline_run.get.load_config")
    def test_defaults(self, mock_load):
        """Missing key → default OSS label."""
        mock_load.return_value = {}
        self.assertEqual(_env_label(), _DEFAULT_ENV_LABEL)

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline_run.get.load_config")
    def test_override(self, mock_load):
        """Config override returned as-is."""
        mock_load.return_value = {"pipeline_run_environment_label": "internal/env"}
        self.assertEqual(_env_label(), "internal/env")


class RenderRevisionTest(TestCase):
    """REVISION column falls back through revision → draft → empty."""

    def _run(self, revision_name=None, draft_name=None):
        """Build a PipelineRun mock with the given revision + draft state."""
        spec = MagicMock()

        def _has_field(f):
            return (f == "revision" and revision_name is not None) or (
                f == "draft" and draft_name is not None
            )

        spec.HasField.side_effect = _has_field
        if revision_name is not None:
            spec.revision.name = revision_name
        if draft_name is not None:
            spec.draft.name = draft_name
        return SimpleNamespace(spec=spec)

    def test_revision_present(self):
        """Populated revision renders its name (draft ignored)."""
        self.assertEqual(_render_revision(self._run("rev-abc", "draft-xyz")), "rev-abc")

    def test_revision_missing_draft_present(self):
        """Missing revision → falls back to draft name."""
        self.assertEqual(_render_revision(self._run(None, "draft-xyz")), "draft-xyz")

    def test_both_missing(self):
        """Neither set → empty string."""
        self.assertEqual(_render_revision(self._run(None, None)), "")

    def test_revision_present_but_empty_name(self):
        """Revision set with empty name → falls back to draft."""
        self.assertEqual(_render_revision(self._run("", "draft-xyz")), "draft-xyz")


class RenderUserTest(TestCase):
    """USER column reads spec.actor.name with empty-string fallback."""

    def _run(self, actor_name=None):
        """Build a PipelineRun mock; spec.actor set iff actor_name is not None."""
        spec = MagicMock()
        spec.HasField.side_effect = lambda f: f == "actor" and actor_name is not None
        if actor_name is not None:
            spec.actor.name = actor_name
        return SimpleNamespace(spec=spec)

    def test_actor_set(self):
        """Populated actor renders as its name."""
        self.assertEqual(_render_user(self._run("alice")), "alice")

    def test_actor_missing(self):
        """Missing spec.actor renders as empty."""
        self.assertEqual(_render_user(self._run(None)), "")


class RenderEnvTest(TestCase):
    """ENVIRONMENT column reads the configured label from metadata.labels."""

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline_run.get._env_label")
    def test_label_present(self, mock_label):
        """Configured label present → value returned."""
        mock_label.return_value = "michelangelo/environment"
        item = SimpleNamespace(
            metadata=SimpleNamespace(labels={"pipelinerun.michelangelo/environment": "prod"})
        )
        self.assertEqual(_render_env(item), "prod")

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline_run.get._env_label")
    def test_label_missing(self, mock_label):
        """Configured label absent → empty string."""
        mock_label.return_value = "michelangelo/environment"
        item = SimpleNamespace(metadata=SimpleNamespace(labels={}))
        self.assertEqual(_render_env(item), "")


class RenderStateTest(TestCase):
    """STATE column strips PIPELINE_RUN_STATE_ prefix and falls back to numeric."""

    def setUp(self):
        """Pin the plugin's pb2 loader to the OSS 5-value enum stub."""
        patcher = patch(
            "michelangelo.cli.mactl.plugins.entity.pipeline_run.get._load_state_pb2",
            return_value=_stub_pb2(_STATE_OSS),
        )
        self.mock_load = patcher.start()
        self.addCleanup(patcher.stop)

    def test_prefix_stripped(self):
        """PENDING = 1 → short 'PENDING'."""
        item = SimpleNamespace(status=SimpleNamespace(state=1))
        self.assertEqual(_render_state(item), "PENDING")

    def test_running(self):
        """RUNNING = 2 → 'RUNNING'."""
        item = SimpleNamespace(status=SimpleNamespace(state=2))
        self.assertEqual(_render_state(item), "RUNNING")

    def test_unknown_falls_back_to_numeric(self):
        """State int not in configured pb2 → numeric string, no crash."""
        item = SimpleNamespace(status=SimpleNamespace(state=99))
        self.assertEqual(_render_state(item), "99")


class AddGetFiltersTest(TestCase):
    """CRD hook wiring — args, columns, and per-field operator in filter_field_map."""

    def _fresh_crd(self):
        """CRD-like namespace with the three list/dict slots CRD.__init__ creates."""
        return SimpleNamespace(
            additional_get_args=[],
            filter_field_map={},
            additional_columns=[],
        )

    def test_registers_two_args(self):
        """--actor and --revision are added to additional_get_args."""
        crd = self._fresh_crd()
        add_get_filters(crd)
        dests = [a["kwargs"]["dest"] for a in crd.additional_get_args]
        self.assertEqual(dests, ["actor", "revision"])

    def test_filter_field_map_actor_is_str_default_equal(self):
        """--actor entry is a plain string → defaults to EQUAL in _list_func_impl."""
        crd = self._fresh_crd()
        add_get_filters(crd)
        self.assertEqual(crd.filter_field_map["actor"], "pipeline_run.spec.actor.name")

    def test_filter_field_map_revision_carries_like_operator(self):
        """--revision entry is a dict with explicit LIKE operator."""
        crd = self._fresh_crd()
        add_get_filters(crd)
        self.assertEqual(
            crd.filter_field_map["revision"],
            {
                "field": "pipeline_run.spec.revision.name",
                "operator": _CRITERION_OPERATOR_LIKE,
            },
        )

    def test_registers_four_columns(self):
        """REVISION, USER, ENVIRONMENT, STATE columns registered in order."""
        crd = self._fresh_crd()
        add_get_filters(crd)
        self.assertEqual(
            [c["column_name"] for c in crd.additional_columns],
            ["REVISION", "USER", "ENVIRONMENT", "STATE"],
        )
