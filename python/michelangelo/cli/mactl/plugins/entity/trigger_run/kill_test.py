"""Unit tests for trigger_run kill command.

Focused on the F025 dry-run addition: signature contains the flag, the helper
is invoked with `update_options`, and the early-return branch skips the
post-RPC verify when `dry_run=True`.
"""

from unittest import TestCase
from unittest.mock import MagicMock, Mock, patch

from michelangelo.cli.mactl.crd import CRD
from michelangelo.cli.mactl.plugins.entity.pipeline.kill_test import (
    _reflection_built,
)
from michelangelo.cli.mactl.plugins.entity.trigger_run.kill import (
    add_function_signature,
    generate_kill,
)
from michelangelo.gen.api.v2 import trigger_run_svc_pb2


def _get_response(has_spec: bool = True) -> MagicMock:
    """A Get*Response stand-in whose resource reports a populated spec.

    kill_func copies the resource proto across instead of round-tripping it
    through a dict, so it asks the response for the field directly.
    """
    resource = MagicMock()
    resource.HasField.return_value = has_spec
    response = MagicMock()
    response.trigger_run = resource
    return response


def _update_response(kill: bool = True) -> MagicMock:
    """An Update*Response stand-in whose resource reports `spec.kill`."""
    response = MagicMock()
    response.trigger_run.spec.kill = kill
    return response


def _make_crd_mock():
    mock_crd = Mock(spec=CRD)
    mock_crd.name = "trigger_run"
    mock_crd.full_name = "michelangelo.api.v2.TriggerRunService"
    mock_crd.metadata = []
    mock_crd.func_signature = {}

    mock_signature = Mock()

    def _bind(*args, **kwargs):
        bound = Mock()
        bound.arguments = {
            "self": args[0] if args else kwargs.get("self"),
            "namespace": kwargs.get("namespace"),
            "name": kwargs.get("name"),
            "yes": kwargs.get("yes", False),
            "dry_run": kwargs.get("dry_run", False),
        }
        return bound

    mock_signature.bind = _bind
    mock_crd._read_signatures = Mock(return_value=mock_signature)
    mock_crd.configure_parser = Mock()
    return mock_crd


class TriggerRunKillSignatureTest(TestCase):
    """add_function_signature must include the dry_run arg."""

    def test_signature_contains_dry_run(self):
        """Signature declares a dry_run flag on trigger_run kill."""
        mock_crd = Mock()
        mock_crd.func_signature = {}

        add_function_signature(mock_crd)

        dests = [
            a["kwargs"].get("dest") or a["args"][-1].lstrip("-").replace("-", "_")
            for a in mock_crd.func_signature["kill"]["args"]
        ]
        self.assertIn("dry_run", dests)


class TriggerRunKillDryRunTest(TestCase):
    """kill_func must apply dry_run and early-return when the flag is set."""

    @patch(
        "michelangelo.cli.mactl.plugins.entity.trigger_run.kill.crd_module.apply_dry_run_to_request"
    )
    def test_dry_run_forwards_to_helper_and_early_returns(self, mock_apply_dry):
        """Dry-run: helper called with `update_options`; post-RPC verify skipped."""
        mock_crd = _make_crd_mock()
        mock_channel = Mock()

        mock_input_class = MagicMock()
        mock_output_class = MagicMock()
        mock_crd._extract_method_info = Mock(
            return_value=("UpdateTriggerRun", mock_input_class, mock_output_class)
        )
        mock_crd.get = Mock(return_value=_get_response())

        # spec.kill false in the response: only the early return keeps this from
        # failing verification.
        mock_stub = Mock(return_value=_update_response(kill=False))
        mock_channel.unary_unary.return_value = mock_stub

        generate_kill(mock_crd, mock_channel)
        result = mock_crd.kill(
            mock_crd, namespace="ns", name="run", yes=True, dry_run=True
        )

        # helper invoked with update_options
        args = mock_apply_dry.call_args[0]
        self.assertEqual(args[1], "update_options")
        self.assertTrue(args[2]["dry_run"])

        self.assertIsNotNone(result)

    def test_no_dry_run_runs_full_verify_branch(self):
        """Without dry_run: a response with spec.kill set is returned."""
        mock_crd = _make_crd_mock()
        mock_channel = Mock()

        mock_crd._extract_method_info = Mock(
            return_value=("UpdateTriggerRun", MagicMock(), MagicMock())
        )
        mock_crd.get = Mock(return_value=_get_response())
        updated = _update_response()
        mock_channel.unary_unary.return_value = Mock(return_value=updated)

        generate_kill(mock_crd, mock_channel)
        result = mock_crd.kill(
            mock_crd, namespace="ns", name="run", yes=True, dry_run=False
        )

        self.assertIs(result, updated)

    def test_kill_flag_not_set_in_response_raises(self):
        """Without dry_run: a response with spec.kill unset fails verification."""
        mock_crd = _make_crd_mock()
        mock_channel = Mock()

        mock_crd._extract_method_info = Mock(
            return_value=("UpdateTriggerRun", MagicMock(), MagicMock())
        )
        mock_crd.get = Mock(return_value=_get_response())

        mock_stub = Mock(return_value=_update_response(kill=False))
        mock_channel.unary_unary.return_value = mock_stub

        generate_kill(mock_crd, mock_channel)
        with self.assertRaises(RuntimeError) as ctx:
            mock_crd.kill(mock_crd, namespace="ns", name="run", yes=True, dry_run=False)

        self.assertIn("Kill operation failed", str(ctx.exception))

    def test_missing_spec_field_is_refused(self):
        """A resource with no spec cannot be killed."""
        mock_crd = _make_crd_mock()
        mock_channel = Mock()
        mock_crd._extract_method_info = Mock(
            return_value=("UpdateTriggerRun", MagicMock(), MagicMock())
        )
        mock_crd.get = Mock(return_value=_get_response(has_spec=False))

        generate_kill(mock_crd, mock_channel)
        with self.assertRaises(ValueError) as ctx:
            mock_crd.kill(mock_crd, namespace="ns", name="run", yes=True, dry_run=False)

        self.assertIn("Cannot set kill flag", str(ctx.exception))


class TriggerRunKillProtoCopyTest(TestCase):
    """kill_func must copy the fetched TriggerRun into the request faithfully.

    The fetched resource is built in its own descriptor pool, as it is at
    runtime, so it is a distinct class from the Update request field. A field
    this client does not know must still reach the server: the old dict round
    trip dropped such fields silently.
    """

    # Field 999 (length-delimited) carrying b"abc"; absent from TriggerRun.
    unknown_field = b"\xba\x3e\x03abc"

    def test_fetched_resource_reaches_the_update_request_intact(self):
        """Known and unknown fields survive; spec.kill is set and verified."""
        fetched = _reflection_built(trigger_run_svc_pb2.GetTriggerRunResponse)()
        fetched.trigger_run.metadata.name = "run"
        fetched.trigger_run.metadata.namespace = "ns"
        fetched.trigger_run.spec.kill = False
        fetched.trigger_run.MergeFromString(self.unknown_field)

        updated = _reflection_built(trigger_run_svc_pb2.UpdateTriggerRunResponse)()
        updated.trigger_run.spec.kill = True

        mock_crd = _make_crd_mock()
        mock_crd._extract_method_info = Mock(
            return_value=(
                "UpdateTriggerRun",
                trigger_run_svc_pb2.UpdateTriggerRunRequest,
                trigger_run_svc_pb2.UpdateTriggerRunResponse,
            )
        )
        mock_crd.get = Mock(return_value=fetched)
        stub = Mock(return_value=updated)
        mock_channel = Mock()
        mock_channel.unary_unary.return_value = stub

        generate_kill(mock_crd, mock_channel)
        result = mock_crd.kill(
            mock_crd, namespace="ns", name="run", yes=True, dry_run=False
        )

        sent = stub.call_args[0][0].trigger_run
        self.assertTrue(sent.spec.kill)
        self.assertEqual(sent.metadata.name, "run")
        self.assertEqual(sent.metadata.namespace, "ns")
        self.assertIn(self.unknown_field, sent.SerializeToString())
        self.assertIs(result, updated)
