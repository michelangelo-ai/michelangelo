"""Unit tests for pipeline get filters + display columns."""

from types import SimpleNamespace
from unittest import TestCase
from unittest.mock import MagicMock, patch

from michelangelo.cli.mactl.plugins.entity.pipeline.get import (
    _DEFAULT_PB2_MODULE,
    _load_pipeline_pb2,
    _normalize_pipeline_type,
    _render_owner,
    _render_type,
    add_get_filters,
)


class LoadPipelinePb2Test(TestCase):
    """The proto module path is configurable via ``pipeline_type_pb2_module``."""

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.get.importlib.import_module")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.get.load_config")
    def test_defaults_to_v2_when_config_absent(self, mock_load, mock_import):
        """Missing key → uses the default OSS v2 module path."""
        mock_load.return_value = {}
        _load_pipeline_pb2()
        mock_import.assert_called_once_with(_DEFAULT_PB2_MODULE)

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.get.importlib.import_module")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.get.load_config")
    def test_honors_config_override(self, mock_load, mock_import):
        """Config value is passed straight to importlib.import_module."""
        mock_load.return_value = {
            "pipeline_type_pb2_module": "downstream.pkg.pipeline_pb2"
        }
        _load_pipeline_pb2()
        mock_import.assert_called_once_with("downstream.pkg.pipeline_pb2")


def _stub_pb2(names_with_ids):
    """Build a stand-in for a pipeline_pb2 module carrying the given enum."""
    stub = MagicMock()
    stub.PipelineType.DESCRIPTOR.values = [
        SimpleNamespace(name=n, number=i) for n, i in names_with_ids
    ]

    def _name(value):
        for n, i in names_with_ids:
            if i == value:
                return n
        raise ValueError(f"{value} is not a valid PipelineType")

    stub.PipelineType.Name.side_effect = _name
    return stub


_OSS_ENUM = [
    ("PIPELINE_TYPE_INVALID", 0),
    ("PIPELINE_TYPE_TRAIN", 1),
    ("PIPELINE_TYPE_EVAL", 2),
]
_EXTENDED_ENUM = [*_OSS_ENUM, ("PIPELINE_TYPE_TRAIN_LLM", 18)]


class NormalizePipelineTypeTest(TestCase):
    """--type flag value normalization uses whichever pb2 is configured."""

    def setUp(self):
        """Pin the plugin's pb2 loader to the OSS 3-value enum stub."""
        patcher = patch(
            "michelangelo.cli.mactl.plugins.entity.pipeline.get._load_pipeline_pb2",
            return_value=_stub_pb2(_OSS_ENUM),
        )
        self.mock_load = patcher.start()
        self.addCleanup(patcher.stop)

    def test_short_name_uppercase(self):
        """Short name in upper case gets the PIPELINE_TYPE_ prefix."""
        self.assertEqual(_normalize_pipeline_type("TRAIN"), "PIPELINE_TYPE_TRAIN")

    def test_short_name_lowercase_normalizes(self):
        """Lower-case short name is upper-cased then prefixed."""
        self.assertEqual(_normalize_pipeline_type("train"), "PIPELINE_TYPE_TRAIN")

    def test_full_name_passes_through(self):
        """Already-prefixed value returns unchanged."""
        self.assertEqual(
            _normalize_pipeline_type("PIPELINE_TYPE_EVAL"), "PIPELINE_TYPE_EVAL"
        )

    def test_whitespace_stripped(self):
        """Surrounding whitespace is trimmed before normalization."""
        self.assertEqual(_normalize_pipeline_type("  train  "), "PIPELINE_TYPE_TRAIN")

    def test_unknown_raises_with_valid_list(self):
        """Unknown value raises ValueError listing the accepted names."""
        with self.assertRaises(ValueError) as cm:
            _normalize_pipeline_type("BOGUS")
        msg = str(cm.exception)
        self.assertIn("BOGUS", msg)
        self.assertIn("TRAIN", msg)
        self.assertNotIn("INVALID", msg)


class NormalizeAcceptsExtendedEnumTest(TestCase):
    """A downstream pb2 with extra values (e.g. TRAIN_LLM) is accepted."""

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.get._load_pipeline_pb2")
    def test_extended_value_accepted(self, mock_load):
        """Configured pb2 exposing TRAIN_LLM → --type TRAIN_LLM validates."""
        mock_load.return_value = _stub_pb2(_EXTENDED_ENUM)
        self.assertEqual(
            _normalize_pipeline_type("TRAIN_LLM"), "PIPELINE_TYPE_TRAIN_LLM"
        )


class RenderOwnerTest(TestCase):
    """OWNER column value function."""

    def _pipeline(self, owner_name=None):
        """Build a Pipeline mock whose spec.owner is set iff owner_name is not None."""
        spec = MagicMock()
        spec.HasField.side_effect = lambda f: f == "owner" and owner_name is not None
        if owner_name is not None:
            spec.owner.name = owner_name
        return SimpleNamespace(spec=spec)

    def test_owner_set(self):
        """Populated owner renders as its name."""
        self.assertEqual(_render_owner(self._pipeline("alice")), "alice")

    def test_owner_unset_returns_empty(self):
        """Missing spec.owner renders as empty string."""
        self.assertEqual(_render_owner(self._pipeline(None)), "")

    def test_owner_empty_string_returns_empty(self):
        """Owner set with an empty name still renders empty."""
        self.assertEqual(_render_owner(self._pipeline("")), "")


class RenderTypeTest(TestCase):
    """TYPE column value function."""

    def setUp(self):
        """Pin the plugin's pb2 loader to the OSS 3-value enum stub."""
        patcher = patch(
            "michelangelo.cli.mactl.plugins.entity.pipeline.get._load_pipeline_pb2",
            return_value=_stub_pb2(_OSS_ENUM),
        )
        self.mock_load = patcher.start()
        self.addCleanup(patcher.stop)

    def test_strips_prefix(self):
        """PipelineType.TRAIN = 1 → short name 'TRAIN'."""
        item = SimpleNamespace(spec=SimpleNamespace(type=1))
        self.assertEqual(_render_type(item), "TRAIN")

    def test_unknown_enum_value_renders_as_numeric_fallback(self):
        """Configured pb2 without this value → numeric fallback, no exception."""
        item = SimpleNamespace(spec=SimpleNamespace(type=42))
        self.assertEqual(_render_type(item), "42")


class AddGetFiltersTest(TestCase):
    """CRD hook wiring — additional_get_args, filter_field_map, additional_columns."""

    def _fresh_crd(self):
        """CRD-like namespace with the three list/dict slots CRD.__init__ creates."""
        return SimpleNamespace(
            additional_get_args=[],
            filter_field_map={},
            additional_columns=[],
        )

    def test_registers_two_args_two_columns_two_filter_fields(self):
        """add_get_filters populates all three CRD hook slots with matched entries."""
        crd = self._fresh_crd()
        add_get_filters(crd)

        self.assertEqual(len(crd.additional_get_args), 2)
        dests = [a["kwargs"]["dest"] for a in crd.additional_get_args]
        self.assertEqual(dests, ["owner", "type"])

        self.assertEqual(
            crd.filter_field_map,
            {
                "owner": "pipeline.spec.owner.name",
                "type": "pipeline.spec.type",
            },
        )

        self.assertEqual(
            [c["column_name"] for c in crd.additional_columns],
            ["OWNER", "TYPE"],
        )

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.get._load_pipeline_pb2",
        return_value=_stub_pb2(_OSS_ENUM),
    )
    def test_type_arg_uses_normalizer_as_argparse_type(self, _mock_load):
        """--type registers _normalize_pipeline_type as its argparse type callable."""
        crd = self._fresh_crd()
        add_get_filters(crd)
        type_arg = next(
            a for a in crd.additional_get_args if a["kwargs"]["dest"] == "type"
        )
        normalize = type_arg["kwargs"]["type"]
        self.assertEqual(normalize("train"), "PIPELINE_TYPE_TRAIN")
        with self.assertRaises(ValueError):
            normalize("BOGUS")

    def test_owner_arg_default_empty(self):
        """--owner defaults to empty string and is not required."""
        crd = self._fresh_crd()
        add_get_filters(crd)
        owner_arg = next(
            a for a in crd.additional_get_args if a["kwargs"]["dest"] == "owner"
        )
        self.assertEqual(owner_arg["kwargs"]["default"], "")
        self.assertFalse(owner_arg["kwargs"]["required"])
