"""Unit tests for pipeline get filters + display columns."""

from types import SimpleNamespace
from unittest import TestCase
from unittest.mock import MagicMock

from michelangelo.cli.mactl.plugins.entity.pipeline.get import (
    _normalize_pipeline_type,
    _render_owner,
    _render_type,
    add_get_filters,
)


class NormalizePipelineTypeTest(TestCase):
    """--type flag value normalization."""

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

    def test_invalid_sentinel_accepted(self):
        """PIPELINE_TYPE_INVALID is a real enum value so normalize accepts it."""
        self.assertEqual(_normalize_pipeline_type("INVALID"), "PIPELINE_TYPE_INVALID")


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

    def test_strips_prefix(self):
        """PipelineType.TRAIN = 1 → short name 'TRAIN'."""
        item = SimpleNamespace(spec=SimpleNamespace(type=1))
        self.assertEqual(_render_type(item), "TRAIN")

    def test_invalid_zero_renders_as_invalid_short(self):
        """PipelineType.INVALID = 0 → 'INVALID' after prefix strip."""
        item = SimpleNamespace(spec=SimpleNamespace(type=0))
        self.assertEqual(_render_type(item), "INVALID")


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

    def test_type_arg_uses_normalizer_as_argparse_type(self):
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
