"""Pipeline `apply` function plugin module."""

from logging import getLogger
from pathlib import Path

from git import Repo
from google.protobuf.message import Message
from grpc import RpcError, StatusCode

from michelangelo.cli.mactl.crd import (
    CRD,
    CrdMethodInfo,
    crd_method_call,
    crd_method_call_kwargs,
    get_crd_namespace_and_name_from_yaml,
    read_yaml_to_crd_request,
)
from michelangelo.cli.mactl.plugins.entity.pipeline.create import (
    handle_workflow_inputs_retrieval,
    populate_pipeline_spec_with_workflow_inputs,
)

_LOG = getLogger(__name__)


def convert_crd_metadata_pipeline_apply(
    yaml_dict: dict, crd_class: type[Message], yaml_path: Path
) -> dict:
    """Convert CRD metadata for pipeline apply (update path).

    Runs the same registration subprocess as create to produce an enriched
    spec with a full repo-relative filePath, fresh commit info, owner, and
    uniflow artifacts.

    Returns a full desired-state dict including metadata (name, namespace,
    annotations, labels from the yaml) and spec. uid/resourceVersion are
    intentionally omitted — the caller copies resourceVersion from the
    existing pipeline for optimistic concurrency.
    """
    _LOG.info("Convert CRD metadata for class %r", crd_class)
    if not isinstance(yaml_dict, dict):
        _LOG.error("Expected a dictionary, got: %r", type(yaml_dict))
        raise ValueError("Expected a dictionary for CRD metadata")

    repo = Repo(".", search_parent_directories=True)
    repo_root = Path(repo.git.rev_parse("--show-toplevel")).resolve()
    _LOG.info("Current git repository info: %r", repo)

    project = yaml_dict["metadata"]["namespace"]
    pipeline = yaml_dict["metadata"]["name"]
    config_file_relative_path = str(yaml_path.relative_to(repo_root))

    workflow_inputs, uniflow_tar_path, workflow_function_name = (
        handle_workflow_inputs_retrieval(
            repo_root, config_file_relative_path, project, pipeline
        )
    )

    # Include user-defined metadata from the yaml (name, namespace, annotations,
    # labels). uid/resourceVersion/creationTimestamp are server-managed and not
    # yaml, so they are not included here — the caller is responsible for copying
    # resourceVersion from the existing pipeline for optimistic concurrency.
    res = {
        "metadata": {
            "name": yaml_dict["metadata"]["name"],
            "namespace": yaml_dict["metadata"]["namespace"],
            "annotations": yaml_dict["metadata"].get("annotations", {}),
            "labels": yaml_dict["metadata"].get("labels", {}),
        }
    }
    populate_pipeline_spec_with_workflow_inputs(
        res,
        yaml_dict,
        workflow_inputs,
        repo,
        yaml_path,
        repo_root,
        config_file_relative_path,
        uniflow_tar_path,
        workflow_function_name,
    )
    return res


def pipeline_apply_func_impl(
    get_method_info: CrdMethodInfo,
    update_method_info: CrdMethodInfo,
    bound_args,
) -> Message:
    """Pipeline apply implementation with silent get (no print side-effect).

    get_method_info is captured via partial in apply_plugin_command so that
    the existence check bypasses get_func_impl's print side-effect.
    update_method_info is passed by generate_apply via the standard partial mechanism.
    """
    _self: CRD = bound_args.arguments["self"]
    _file = bound_args.arguments["file"]

    _namespace, _name = get_crd_namespace_and_name_from_yaml(_file)

    message_instance = None
    try:
        message_instance = crd_method_call_kwargs(
            get_method_info,
            namespace=_namespace,
            name=_name,
        )
    except RpcError as err:
        _LOG.debug("Pipeline %r / %r does not exist: %r", _namespace, _name, err)
        if err.code() != StatusCode.NOT_FOUND:
            raise

    if message_instance is None:
        _LOG.info("Create a new pipeline")
        _self.generate_create(update_method_info.channel)
        original_converter = _self.func_crd_metadata_converter
        create_converter = getattr(
            _self, "func_crd_metadata_converter_for_create", original_converter
        )
        _self.func_crd_metadata_converter = create_converter
        try:
            return _self.create(_file)
        finally:
            _self.func_crd_metadata_converter = original_converter

    _LOG.info("Updating existing pipeline: %r", message_instance)
    converter = _self.func_crd_metadata_converter
    request_input = read_yaml_to_crd_request(
        update_method_info.input_class,
        _self.name,
        _file,
        converter,
    )
    existing = getattr(message_instance, _self.name)
    inner = getattr(request_input, _self.name)
    inner.metadata.resourceVersion = existing.metadata.resourceVersion
    call_res = crd_method_call(update_method_info, request_input)
    print(call_res)
    return call_res
