"""Unit tests for pipeline apply plugin."""

from pathlib import Path
from unittest import TestCase
from unittest.mock import MagicMock, Mock, patch

from grpc import RpcError, StatusCode

from michelangelo.cli.mactl.plugins.entity.pipeline.apply import (
    convert_crd_metadata_pipeline_apply,
    pipeline_apply_func_impl,
)


class _FakeRpcError(RpcError):
    def __init__(self, code):
        self._code = code

    def code(self):
        return self._code


def _make_yaml_dict(spec=None):
    return {
        "apiVersion": "michelangelo.api/v2",
        "kind": "Pipeline",
        "metadata": {"name": "my-pipeline", "namespace": "my-project"},
        "spec": spec
        or {
            "type": "PIPELINE_TYPE_TRAIN",
            "manifest": {"filePath": "examples.my_pipeline.workflow"},
        },
    }


def _make_mock_repo(sha="abc123def456", branch="main", repo_root="/repo"):
    mock_repo = Mock()
    mock_repo.active_branch.name = branch
    mock_repo.head.commit.hexsha = sha
    mock_repo.git.rev_parse.return_value = repo_root
    return mock_repo


class PipelineApplyTest(TestCase):
    """Tests for convert_crd_metadata_pipeline_apply."""

    def _patch_repo(self, mock_repo):
        return patch(
            "michelangelo.cli.mactl.plugins.entity.pipeline.apply.Repo",
            return_value=mock_repo,
        )

    def _patch_handle(self, return_value=(None, "", "")):
        return patch(
            "michelangelo.cli.mactl.plugins.entity.pipeline.apply"
            ".handle_workflow_inputs_retrieval",
            return_value=return_value,
        )

    def _patch_populate(self, return_value=None):
        if return_value is None:
            return_value = {
                "spec": {"manifest": {"filePath": "full/path/pipeline.yaml"}}
            }
        return patch(
            "michelangelo.cli.mactl.plugins.entity.pipeline.apply"
            ".populate_pipeline_spec_with_workflow_inputs",
            return_value=return_value,
        )

    # ------------------------------------------------------------------
    # Registration is called
    # ------------------------------------------------------------------

    def test_registration_is_called(self):
        """Handle_workflow_inputs_retrieval is called with project and pipeline."""
        yaml_dict = _make_yaml_dict()
        yaml_path = Path("/repo/my-project/my-pipeline/pipeline.yaml")
        mock_repo = _make_mock_repo(repo_root="/repo")

        with (
            self._patch_repo(mock_repo),
            self._patch_handle() as mock_handle,
            self._patch_populate(),
        ):
            convert_crd_metadata_pipeline_apply(yaml_dict, Mock(), yaml_path)

        mock_handle.assert_called_once()
        call_args = mock_handle.call_args
        # project and pipeline come from yaml metadata
        args = call_args[0]
        self.assertEqual(args[2], "my-project")  # project
        self.assertEqual(args[3], "my-pipeline")  # pipeline
        # config_file_relative_path is relative to repo root
        self.assertIn("my-project/my-pipeline/pipeline.yaml", args[1])

    # ------------------------------------------------------------------
    # filePath is set to the full repo-relative path
    # ------------------------------------------------------------------

    def test_file_path_set_to_repo_relative_path(self):
        """Populate receives the full repo-relative path."""
        yaml_dict = _make_yaml_dict()
        yaml_path = Path("/repo/my-project/my-pipeline/pipeline.yaml")
        mock_repo = _make_mock_repo(repo_root="/repo")

        with (
            self._patch_repo(mock_repo),
            self._patch_handle(),
            self._patch_populate() as mock_populate,
        ):
            convert_crd_metadata_pipeline_apply(yaml_dict, Mock(), yaml_path)

        # 6th positional arg is config_file_relative_path
        config_rel_path = mock_populate.call_args[0][6]
        self.assertEqual(config_rel_path, "my-project/my-pipeline/pipeline.yaml")

    # ------------------------------------------------------------------
    # Commit info forwarded to populate
    # ------------------------------------------------------------------

    def test_commit_info_forwarded(self):
        """The repo object (carrying commit SHA and branch) is passed to populate."""
        yaml_dict = _make_yaml_dict()
        yaml_path = Path("/repo/pipeline.yaml")
        mock_repo = _make_mock_repo(
            sha="deadbeef", branch="feature/x", repo_root="/repo"
        )

        with (
            self._patch_repo(mock_repo),
            self._patch_handle(),
            self._patch_populate() as mock_populate,
        ):
            convert_crd_metadata_pipeline_apply(yaml_dict, Mock(), yaml_path)

        # 4th positional arg to populate is the repo object
        repo_arg = mock_populate.call_args[0][3]
        self.assertEqual(repo_arg.active_branch.name, "feature/x")
        self.assertEqual(repo_arg.head.commit.hexsha, "deadbeef")

    # ------------------------------------------------------------------
    # Uniflow artifacts forwarded on success
    # ------------------------------------------------------------------

    def test_uniflow_artifacts_forwarded_on_success(self):
        """Uniflow tar and workflow function name are passed through to populate."""
        yaml_dict = _make_yaml_dict()
        yaml_path = Path("/repo/pipeline.yaml")
        mock_repo = _make_mock_repo(repo_root="/repo")

        fake_workflow_inputs = Mock()
        fake_tar = "s3://bucket/my.tar.gz"
        fake_fn = "my_workflow"

        with (
            self._patch_repo(mock_repo),
            self._patch_handle(return_value=(fake_workflow_inputs, fake_tar, fake_fn)),
            self._patch_populate() as mock_populate,
        ):
            convert_crd_metadata_pipeline_apply(yaml_dict, Mock(), yaml_path)

        args = mock_populate.call_args[0]
        self.assertEqual(args[2], fake_workflow_inputs)  # workflow_inputs
        self.assertEqual(args[7], fake_tar)  # uniflow_tar_path
        self.assertEqual(args[8], fake_fn)  # workflow_function_name

    # ------------------------------------------------------------------
    # Graceful degradation: filePath still set when registration fails
    # ------------------------------------------------------------------

    def test_graceful_degradation_on_registration_failure(self):
        """Populate is still called with empty tar/fn when registration fails."""
        yaml_dict = _make_yaml_dict()
        yaml_path = Path("/repo/pipeline.yaml")
        mock_repo = _make_mock_repo(repo_root="/repo")

        # handle returns empty strings — the graceful-degradation case
        with (
            self._patch_repo(mock_repo),
            self._patch_handle(return_value=(None, "", "")),
            self._patch_populate() as mock_populate,
        ):
            convert_crd_metadata_pipeline_apply(yaml_dict, Mock(), yaml_path)

        mock_populate.assert_called_once()
        args = mock_populate.call_args[0]
        self.assertIsNone(args[2])  # workflow_inputs
        self.assertEqual(args[7], "")  # uniflow_tar_path
        self.assertEqual(args[8], "")  # workflow_function_name

    # ------------------------------------------------------------------
    # Full metadata is included (for full-replace semantics)
    # ------------------------------------------------------------------

    def test_metadata_included_in_result(self):
        """Result must contain metadata with name, namespace, annotations from yaml."""
        yaml_dict = _make_yaml_dict()
        yaml_path = Path("/repo/my-project/my-pipeline/pipeline.yaml")
        mock_repo = _make_mock_repo(repo_root="/repo")

        def fake_populate(res, *args, **kwargs):
            res["spec"] = {"manifest": {"filePath": "full/path"}}
            return res

        with (
            self._patch_repo(mock_repo),
            self._patch_handle(),
            patch(
                "michelangelo.cli.mactl.plugins.entity.pipeline.apply"
                ".populate_pipeline_spec_with_workflow_inputs",
                side_effect=fake_populate,
            ),
        ):
            result = convert_crd_metadata_pipeline_apply(yaml_dict, Mock(), yaml_path)

        self.assertIn("metadata", result)
        self.assertEqual(result["metadata"]["name"], "my-pipeline")
        self.assertEqual(result["metadata"]["namespace"], "my-project")
        self.assertIn("spec", result)

    def test_annotations_from_yaml_included_in_result(self):
        """Annotations from yaml are included so full-replace preserves metadata."""
        yaml_dict = _make_yaml_dict()
        yaml_dict["metadata"]["annotations"] = {
            "michelangelo/uniflow-image": "docker.io/library/examples:v2"
        }
        yaml_path = Path("/repo/my-project/my-pipeline/pipeline.yaml")
        mock_repo = _make_mock_repo(repo_root="/repo")

        with self._patch_repo(mock_repo), self._patch_handle(), self._patch_populate():
            result = convert_crd_metadata_pipeline_apply(yaml_dict, Mock(), yaml_path)

        self.assertEqual(
            result["metadata"]["annotations"]["michelangelo/uniflow-image"],
            "docker.io/library/examples:v2",
        )

    # ------------------------------------------------------------------
    # Invalid input
    # ------------------------------------------------------------------

    def test_invalid_input_raises_value_error(self):
        """Non-dict input raises ValueError."""
        with self.assertRaises(ValueError) as ctx:
            convert_crd_metadata_pipeline_apply("not a dict", Mock(), Path("/fake"))
        self.assertIn("Expected a dictionary", str(ctx.exception))


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
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.apply.read_yaml_to_crd_request")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.apply.get_crd_namespace_and_name_from_yaml")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.apply.yaml_to_dict")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.apply.crd_method_call_kwargs")
    def test_update_path(self, mock_get, mock_yaml, mock_ns, mock_read_yaml, mock_call):
        """Existing pipeline triggers update path with resourceVersion copy."""
        get_info = self._make_method_info()
        update_info = self._make_method_info()
        mock_yaml.return_value = {"metadata": {"namespace": "ns", "name": "pipe"}}
        mock_ns.return_value = ("ns", "pipe")
        mock_existing = Mock()
        mock_existing.pipeline.metadata.resourceVersion = "42"
        mock_get.return_value = mock_existing
        mock_crd = Mock()
        mock_crd.name = "pipeline"
        mock_request = Mock()
        mock_read_yaml.return_value = mock_request

        pipeline_apply_func_impl(get_info, update_info, self._make_bound_args(mock_crd))

        mock_read_yaml.assert_called_once()
        mock_call.assert_called_once_with(update_info, mock_request)

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.apply.crd_method_call_kwargs")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.apply.get_crd_namespace_and_name_from_yaml")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.apply.yaml_to_dict")
    def test_create_path_when_not_found(self, mock_yaml, mock_ns, mock_get):
        """NOT_FOUND triggers create path."""
        get_info = self._make_method_info()
        update_info = self._make_method_info()
        mock_yaml.return_value = {"metadata": {"namespace": "ns", "name": "pipe"}}
        mock_ns.return_value = ("ns", "pipe")
        mock_get.side_effect = _FakeRpcError(StatusCode.NOT_FOUND)
        mock_crd = Mock()

        pipeline_apply_func_impl(get_info, update_info, self._make_bound_args(mock_crd))

        mock_crd.generate_create.assert_called_once_with(update_info.channel)
        mock_crd.create.assert_called_once_with("f.yaml")

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.apply.crd_method_call_kwargs")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.apply.get_crd_namespace_and_name_from_yaml")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.apply.yaml_to_dict")
    def test_reraises_non_not_found_errors(self, mock_yaml, mock_ns, mock_get):
        """Non-NOT_FOUND RpcErrors are re-raised."""
        get_info = self._make_method_info()
        update_info = self._make_method_info()
        mock_yaml.return_value = {"metadata": {"namespace": "ns", "name": "pipe"}}
        mock_ns.return_value = ("ns", "pipe")
        mock_get.side_effect = _FakeRpcError(StatusCode.UNAVAILABLE)

        with self.assertRaises(RpcError):
            pipeline_apply_func_impl(
                get_info, update_info, self._make_bound_args(Mock())
            )

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.apply.crd_method_call_kwargs")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.apply.get_crd_namespace_and_name_from_yaml")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.apply.yaml_to_dict")
    def test_create_path_uses_create_converter(self, mock_yaml, mock_ns, mock_get):
        """Create path swaps to func_crd_metadata_converter_for_create."""
        get_info = self._make_method_info()
        update_info = self._make_method_info()
        mock_yaml.return_value = {"metadata": {"namespace": "ns", "name": "pipe"}}
        mock_ns.return_value = ("ns", "pipe")
        mock_get.side_effect = _FakeRpcError(StatusCode.NOT_FOUND)
        original = Mock(name="original")
        create_conv = Mock(name="create")
        mock_crd = Mock()
        mock_crd.func_crd_metadata_converter = original
        mock_crd.func_crd_metadata_converter_for_create = create_conv

        pipeline_apply_func_impl(get_info, update_info, self._make_bound_args(mock_crd))

        mock_crd.create.assert_called_once_with("f.yaml")
        self.assertIs(mock_crd.func_crd_metadata_converter, original)
