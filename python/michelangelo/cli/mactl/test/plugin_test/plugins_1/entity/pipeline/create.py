"""Pipeline `create` function plugin module."""

from pathlib import Path

from google.protobuf.message import Message


def convert_crd_metadata_pipeline_create(
    yaml_dict: dict, crd_class: type[Message], yaml_path: Path
) -> dict:
    """Convert CRD metadata for pipeline create crd.

    Integrates pipeline registration to get uniflow artifacts.
    """
    return {
        "metadata": {
            "yaml_dict": yaml_dict,
            "crd_class": repr(crd_class),
            "yaml_path": str(yaml_path),
        }
    }
