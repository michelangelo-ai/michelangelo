from uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder import find_dependency_files
from uber.ai.michelangelo.sdk.model_manager._private.utils.module_utils import save_module_files
from uber.ai.michelangelo.sdk.model_manager._private.serde.loader.custom_model_loader import load_custom_model


def serialize_model_loader(target_dir: str):
    """
    Serialize the custom model loader and its dependencies to the target dir.
    This function is only meant to be used when creating the deployable model package.

    Args:
        target_dir: The target dir to save the module files.
    """
    files = find_dependency_files(load_custom_model.__module__, prefixes=["uber"])
    save_module_files(files, target_dir)
