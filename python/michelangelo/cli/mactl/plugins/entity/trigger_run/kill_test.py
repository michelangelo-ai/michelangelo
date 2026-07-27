"""Unit tests for trigger_run kill command.

Focused on the F025 dry-run addition: signature contains the flag, the helper
is invoked with `update_options`, and the early-return branch skips the
post-RPC verify when `dry_run=True`.
"""

from unittest import TestCase
from unittest.mock import MagicMock, Mock, patch

from google.protobuf.message import Message

from michelangelo.cli.mactl.crd import CRD
from michelangelo.cli.mactl.plugins.entity.trigger_run.kill import (
    add_function_signature,
    generate_kill,
)


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

    @patch("michelangelo.cli.mactl.plugins.entity.trigger_run.kill.MessageToDict")
    @patch("michelangelo.cli.mactl.plugins.entity.trigger_run.kill.ParseDict")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.trigger_run.kill.crd_module.apply_dry_run_to_request"
    )
    def test_dry_run_forwards_to_helper_and_early_returns(
        self, mock_apply_dry, _parse, mock_to_dict
    ):
        """Dry-run: helper called with `update_options`; post-RPC verify skipped."""
        mock_crd = _make_crd_mock()
        mock_channel = Mock()

        mock_input_class = MagicMock()
        mock_output_class = MagicMock()
        mock_crd._extract_method_info = Mock(
            return_value=("UpdateTriggerRun", mock_input_class, mock_output_class)
        )
        mock_crd.get = Mock(return_value=Mock(spec=Message))
        mock_to_dict.return_value = {"trigger_run": {"spec": {"placeholder": "v"}}}

        mock_stub = Mock(return_value=Mock(spec=Message))
        mock_channel.unary_unary.return_value = mock_stub

        generate_kill(mock_crd, mock_channel)
        result = mock_crd.kill(
            mock_crd, namespace="ns", name="run", yes=True, dry_run=True
        )

        # helper invoked with update_options
        args = mock_apply_dry.call_args[0]
        self.assertEqual(args[1], "update_options")
        self.assertTrue(args[2]["dry_run"])

        # RPC returned; MessageToDict was called ONCE (for the input dict), NOT
        # a second time for the response — proves we early-returned instead of
        # entering the "verify spec.kill flipped" branch.
        self.assertEqual(mock_to_dict.call_count, 1)
        self.assertIsNotNone(result)

    @patch("michelangelo.cli.mactl.plugins.entity.trigger_run.kill.MessageToDict")
    @patch("michelangelo.cli.mactl.plugins.entity.trigger_run.kill.ParseDict")
    def test_no_dry_run_runs_full_verify_branch(self, _parse, mock_to_dict):
        """Without dry_run: post-RPC verify branch runs (MessageToDict called twice)."""
        mock_crd = _make_crd_mock()
        mock_channel = Mock()

        mock_crd._extract_method_info = Mock(
            return_value=("UpdateTriggerRun", MagicMock(), MagicMock())
        )
        mock_crd.get = Mock(return_value=Mock(spec=Message))
        mock_to_dict.side_effect = [
            {"trigger_run": {"spec": {"placeholder": "v"}}},
            {"trigger_run": {"spec": {"kill": True}}},
        ]

        mock_stub = Mock(return_value=Mock(spec=Message))
        mock_channel.unary_unary.return_value = mock_stub

        generate_kill(mock_crd, mock_channel)
        mock_crd.kill(mock_crd, namespace="ns", name="run", yes=True, dry_run=False)

        # Verify branch entered: response was serialized to dict → 2 calls total
        self.assertEqual(mock_to_dict.call_count, 2)
