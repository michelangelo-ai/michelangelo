"""Pipeline `apply` function plugin module."""

from logging import getLogger
from pathlib import Path

from git import Repo
from google.protobuf.message import Message

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

    # Include user-defined metadata from the yaml (name, namespace, annotations, labels).
    # uid/resourceVersion/creationTimestamp are server-managed and not present in the yaml,
    # so they are not included here — the caller is responsible for copying resourceVersion
    # from the existing pipeline for optimistic concurrency.
    res = {
        "metadata": {
            "name": yaml_dict["metadata"]["name"],
            "namespace": yaml_dict["metadata"]["namespace"],
            "annotations": yaml_dict.get("metadata", {}).get("annotations", {}),
            "labels": yaml_dict.get("metadata", {}).get("labels", {}),
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
