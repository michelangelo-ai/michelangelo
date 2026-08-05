"""Unit tests for revision get filters + display columns."""

from argparse import ArgumentTypeError
from types import SimpleNamespace
from unittest import TestCase
from unittest.mock import MagicMock

from michelangelo.cli.mactl.plugins.entity.revision.get import (
    _build_type_criteria,
    _render_field,
    add_get_filters,
)

_render_type = _render_field("base_type", "kind")
_render_user = _render_field("owner", "name")
_render_base_resource = _render_field("base_resource", "name")


class BuildTypeCriteriaTest(TestCase):
    """--pipeline / --model / --deployment mutex + composite criteria builder."""

    def test_no_flag_returns_empty(self):
        """No type flag set → no criteria."""
        self.assertEqual(_build_type_criteria({}), [])

    def test_pipeline_with_pattern(self):
        """--pipeline foo → base_type=pipeline + base_resource_name LIKE foo."""
        cs = _build_type_criteria({"pipeline": "foo"})
        self.assertEqual(len(cs), 2)
        self.assertEqual(cs[0]["field"], "revision.spec.base_type.kind")
        self.assertEqual(cs[0]["operator"], 1)
        self.assertEqual(cs[0]["value"], "pipeline")
        self.assertEqual(cs[1]["field"], "revision.spec.base_resource.name")
        self.assertEqual(cs[1]["operator"], 9)
        self.assertEqual(cs[1]["value"], "foo")

    def test_model_with_empty_pattern_uses_percent(self):
        """--model '' → LIKE % fallback matches any name of that type."""
        cs = _build_type_criteria({"model": ""})
        self.assertEqual(cs[0]["value"], "model")
        self.assertEqual(cs[1]["value"], "%")

    def test_deployment(self):
        """--deployment sets base_type to 'deployment'."""
        cs = _build_type_criteria({"deployment": "prod"})
        self.assertEqual(cs[0]["value"], "deployment")
        self.assertEqual(cs[1]["value"], "prod")

    def test_mutex_two_flags_raises(self):
        """Two type flags set at once → ArgumentTypeError."""
        with self.assertRaises(ArgumentTypeError) as cm:
            _build_type_criteria({"pipeline": "a", "model": "b"})
        msg = str(cm.exception)
        self.assertIn("pipeline", msg)
        self.assertIn("model", msg)
        self.assertIn("conflict", msg)

    def test_mutex_three_flags_raises(self):
        """All three type flags set → ArgumentTypeError names all three."""
        with self.assertRaises(ArgumentTypeError):
            _build_type_criteria({"pipeline": "a", "model": "b", "deployment": "c"})

    def test_owner_alone_is_not_a_type_flag(self):
        """--owner is orthogonal — doesn't trigger the mutex or type criteria."""
        self.assertEqual(_build_type_criteria({"owner": "alice"}), [])


class RenderTypeTest(TestCase):
    """TYPE column reads base_type.kind with empty fallback."""

    def _rev(self, kind=None):
        """Build a Revision mock; spec.base_type set iff kind is not None."""
        spec = MagicMock()
        spec.HasField.side_effect = lambda f: f == "base_type" and kind is not None
        if kind is not None:
            spec.base_type.kind = kind
        return SimpleNamespace(spec=spec)

    def test_kind_set(self):
        """base_type.kind returned as-is."""
        self.assertEqual(_render_type(self._rev("pipeline")), "pipeline")

    def test_missing_base_type(self):
        """Unset base_type → empty string."""
        self.assertEqual(_render_type(self._rev(None)), "")

    def test_empty_kind(self):
        """base_type set with empty kind → empty string."""
        self.assertEqual(_render_type(self._rev("")), "")


class RenderUserTest(TestCase):
    """USER column reads owner.name with empty fallback."""

    def _rev(self, name=None):
        """Build a Revision mock; spec.owner set iff name is not None."""
        spec = MagicMock()
        spec.HasField.side_effect = lambda f: f == "owner" and name is not None
        if name is not None:
            spec.owner.name = name
        return SimpleNamespace(spec=spec)

    def test_owner_set(self):
        """owner.name returned as-is."""
        self.assertEqual(_render_user(self._rev("alice")), "alice")

    def test_owner_missing(self):
        """Unset owner → empty string."""
        self.assertEqual(_render_user(self._rev(None)), "")


class RenderBaseResourceTest(TestCase):
    """BASE_RESOURCE column reads base_resource.name with empty fallback."""

    def _rev(self, name=None):
        """Build a Revision mock; spec.base_resource set iff name is not None."""
        spec = MagicMock()
        spec.HasField.side_effect = lambda f: f == "base_resource" and name is not None
        if name is not None:
            spec.base_resource.name = name
        return SimpleNamespace(spec=spec)

    def test_name_set(self):
        """base_resource.name returned as-is."""
        self.assertEqual(_render_base_resource(self._rev("my-pipeline")), "my-pipeline")

    def test_missing(self):
        """Unset base_resource → empty string."""
        self.assertEqual(_render_base_resource(self._rev(None)), "")


class AddGetFiltersTest(TestCase):
    """CRD hook wiring — 4 args, 3 columns, callable + string filter_field_map."""

    def _fresh_crd(self):
        """CRD-like namespace with the three hook slots CRD.__init__ creates."""
        return SimpleNamespace(
            additional_get_args=[],
            filter_field_map={},
            additional_columns=[],
        )

    def test_registers_four_args(self):
        """--pipeline, --model, --deployment, --owner all added in that order."""
        crd = self._fresh_crd()
        add_get_filters(crd)
        dests = [a["kwargs"]["dest"] for a in crd.additional_get_args]
        self.assertEqual(dests, ["pipeline", "model", "deployment", "owner"])

    def test_type_flag_defaults_are_none(self):
        """Type flags default to None so builder distinguishes omitted from empty."""
        crd = self._fresh_crd()
        add_get_filters(crd)
        for arg in crd.additional_get_args:
            if arg["kwargs"]["dest"] in ("pipeline", "model", "deployment"):
                self.assertIsNone(arg["kwargs"]["default"])

    def test_filter_field_map_shape(self):
        """Callable for type group, string for owner."""
        crd = self._fresh_crd()
        add_get_filters(crd)
        self.assertTrue(callable(crd.filter_field_map["_revision_type_group"]))
        self.assertEqual(crd.filter_field_map["owner"], "revision.spec.owner.name")

    def test_registers_three_columns(self):
        """TYPE, USER, BASE_RESOURCE in order."""
        crd = self._fresh_crd()
        add_get_filters(crd)
        self.assertEqual(
            [c["column_name"] for c in crd.additional_columns],
            ["TYPE", "USER", "BASE_RESOURCE"],
        )
