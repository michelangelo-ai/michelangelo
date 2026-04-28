"""Unit tests for pipeline apply plugin."""

from unittest import TestCase
from unittest.mock import MagicMock, Mock, patch

from grpc import RpcError, StatusCode

from michelangelo.cli.mactl.plugins.entity.pipeline.apply import (
    pipeline_apply_func_impl,
)


class _FakeRpcError(RpcError):
    def __init__(self, code):
        self._code = code

    def code(self):
        return self._code


class PipelineApplyFuncImplTest(TestCase):
    """Tests for pipeline_apply_func_impl."""

    def _make_method_info(self):
        from michelangelo.cli.mactl.crd import CrdMethodInfo

        return CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Get",
            input_class=MagicMock(),
            output_class=MagicMock(),
        )

    def _make_bound_args(self, crd, file="f.yaml"):
        return Mock(arguments={"self": crd, "file": file})

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.apply.crd_method_call")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.apply.read_yaml_to_crd_request"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.apply.get_crd_namespace_and_name_from_yaml"
    )
    def test_update_path(self, mock_ns, mock_read_yaml, mock_call):
        """Existing pipeline triggers update path with resourceVersion copy."""
        update_info = self._make_method_info()
        mock_ns.return_value = ("ns", "pipe")
        mock_existing = Mock()
        mock_existing.pipeline.metadata.resourceVersion = "42"
        mock_crd = Mock()
        mock_crd.name = "pipeline"
        mock_crd.get.return_value = mock_existing
        mock_request = Mock()
        mock_read_yaml.return_value = mock_request

        pipeline_apply_func_impl(update_info, self._make_bound_args(mock_crd))

        mock_read_yaml.assert_called_once()
        mock_call.assert_called_once_with(update_info, mock_request)

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.apply.get_crd_namespace_and_name_from_yaml"
    )
    def test_create_path_when_not_found(self, mock_ns):
        """NOT_FOUND triggers create path."""
        update_info = self._make_method_info()
        mock_ns.return_value = ("ns", "pipe")
        mock_crd = Mock()
        mock_crd.get.side_effect = _FakeRpcError(StatusCode.NOT_FOUND)

        pipeline_apply_func_impl(update_info, self._make_bound_args(mock_crd))

        mock_crd.generate_create.assert_called_once_with(update_info.channel)
        mock_crd.create.assert_called_once_with("f.yaml")

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.apply.get_crd_namespace_and_name_from_yaml"
    )
    def test_reraises_non_not_found_errors(self, mock_ns):
        """Non-NOT_FOUND RpcErrors are re-raised."""
        update_info = self._make_method_info()
        mock_ns.return_value = ("ns", "pipe")
        mock_crd = Mock()
        mock_crd.get.side_effect = _FakeRpcError(StatusCode.UNAVAILABLE)

        with self.assertRaises(RpcError):
            pipeline_apply_func_impl(update_info, self._make_bound_args(mock_crd))
