"""Unit tests for mutation_options helpers."""

from types import SimpleNamespace
from unittest import TestCase

from michelangelo.cli.mactl.mutation_options import (
    apply_dry_run_to_request,
    should_emit_metrics,
)


class _RecordingOptions:
    """Mimics k8s.io CreateOptions/UpdateOptions with a dryRun list."""

    def __init__(self):
        self.dryRun = []


def _request(options_attr: str) -> SimpleNamespace:
    """Build a fake request with the given options submessage attr."""
    return SimpleNamespace(**{options_attr: _RecordingOptions()})


class ApplyDryRunToRequestTest(TestCase):
    """apply_dry_run_to_request wiring."""

    def test_dry_run_true_appends_all_to_create_options(self):
        """dry_run=True writes 'All' to create_options.dryRun."""
        req = _request("create_options")
        apply_dry_run_to_request(req, "create_options", {"dry_run": True})
        self.assertEqual(list(req.create_options.dryRun), ["All"])

    def test_dry_run_true_appends_all_to_update_options(self):
        """Same helper works for update_options."""
        req = _request("update_options")
        apply_dry_run_to_request(req, "update_options", {"dry_run": True})
        self.assertEqual(list(req.update_options.dryRun), ["All"])

    def test_dry_run_false_leaves_options_untouched(self):
        """dry_run=False adds nothing (default behavior)."""
        req = _request("create_options")
        apply_dry_run_to_request(req, "create_options", {"dry_run": False})
        self.assertEqual(list(req.create_options.dryRun), [])

    def test_dry_run_missing_leaves_options_untouched(self):
        """No dry_run key in bound_args → no-op."""
        req = _request("update_options")
        apply_dry_run_to_request(req, "update_options", {})
        self.assertEqual(list(req.update_options.dryRun), [])

    def test_dry_run_wire_roundtrip_with_real_proto(self):
        """Round-trip through SerializeToString proves 'dryRun' hits the wire.

        Guards SF-3: writing to `.dry_run` (snake_case) silently no-ops on the
        real proto because k8s.io apimachinery uses camelCase attribute names.
        """
        from google.protobuf.json_format import MessageToDict

        from michelangelo.gen.k8s.io.apimachinery.pkg.apis.meta.v1 import (
            generated_pb2,
        )

        req_wrapper = SimpleNamespace(update_options=generated_pb2.UpdateOptions())
        apply_dry_run_to_request(req_wrapper, "update_options", {"dry_run": True})

        wire = req_wrapper.update_options.SerializeToString()
        parsed = generated_pb2.UpdateOptions.FromString(wire)
        self.assertEqual(
            MessageToDict(parsed, preserving_proto_field_name=False).get("dryRun"),
            ["All"],
        )


class ShouldEmitMetricsTest(TestCase):
    """should_emit_metrics gate."""

    def test_production_no_dry_run_emits(self):
        """Production + no dry-run → True."""
        self.assertTrue(should_emit_metrics({"dry_run": False}, "production"))

    def test_production_dry_run_suppresses(self):
        """Production + dry-run → False."""
        self.assertFalse(should_emit_metrics({"dry_run": True}, "production"))

    def test_non_production_suppresses(self):
        """Any non-production env suppresses even without dry-run."""
        self.assertFalse(should_emit_metrics({"dry_run": False}, "staging"))
