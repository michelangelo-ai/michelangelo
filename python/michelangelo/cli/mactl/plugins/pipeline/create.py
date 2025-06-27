from copy import deepcopy
from logging import getLogger
from os import getenv
from pathlib import Path
from uuid import uuid4

from git import Repo
from google.protobuf.message import Message
from grpc import Channel

from mactl import CRD, PWD


_LOG = getLogger(__name__)


def generate_create(crd: CRD, channel: Channel):
    _LOG.info("Generating `pipeline create` crd for: %s", crd)

    crd.func_crd_metadata_converter = convert_crd_metadata_pipeline_create
    crd.generate_create(channel)


def convert_crd_metadata_pipeline_create(
    yaml_dict: dict, crd_class: type[Message], yaml_path: Path
) -> dict:
    """
    Convert CRD metadata for pipeline create crd.
    """
    _LOG.info("Convert CRD metadata for class %r", crd_class)
    if not isinstance(yaml_dict, dict):
        _LOG.error("Expected a dictionary, got: %r", type(yaml_dict))
        raise ValueError("Expected a dictionary for CRD metadata")

    repo = Repo(".", search_parent_directories=True)
    repo_root = Path(repo.git.rev_parse("--show-toplevel")).resolve()
    _LOG.info("Current git repository info: %r", repo)

    res = {"spec": deepcopy(yaml_dict["spec"])}
    res["metadata"] = {
        "clusterName": "",
        "generateName": "",
        "generation": "0",
        "name": yaml_dict["metadata"]["name"],
        "namespace": yaml_dict["metadata"]["namespace"],
        "resourceVersion": "0",
        "uid": str(uuid4()),
    }
    res["spec"]["commit"] = {
        "branch": repo.active_branch.name,
        "git_ref": repo.head.commit.hexsha,
    }
    assert yaml_path.resolve().is_relative_to(repo_root), (
        f"Expected {yaml_path.resolve()} to be relative to {repo_root}"
    )
    # "path": str(yaml_path.relative_to(PWD)),
    # TODO: retrieve path from Project.
    res["spec"]["manifest"] = {
        "path": "platforms/uberai/michelangelo/ma_integration_test/pipelines/boston_housing/keras_workflow/pipeline.yaml",
        "revision_id": repo.head.commit.hexsha,
        "type": "PIPELINE_MANIFEST_TYPE_YAML",
    }
    res["spec"]["owner"] = {"name": getenv("UBER_LDAP_UID")}
    _LOG.debug("Converted CRD metadata: %r", res)
    return res
