"""Pipeline `apply` function plugin module."""


def convert_crd_metadata_pipeline_apply(*_) -> dict:
    """Convert CRD metadata for pipeline apply crd."""
    return {"test_spec": "plugin_1_test"}
