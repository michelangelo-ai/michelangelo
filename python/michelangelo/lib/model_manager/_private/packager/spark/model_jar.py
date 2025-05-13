from __future__ import annotations
import tempfile
from uber.ai.michelangelo.shared.utils.cmd_utils import execute_cmd, decode_output
from michelangelo.lib.model_manager._private.packager.common import (
    generate_model_package_folder,
)


def create_model_jar(
    model_jar_content: dict,
    model_jar_path: str,
):
    """
    Create a model jar.

    Args:
        model_jar_content: The content of the model jar.
        model_jar_path: The path to save the model jar.

    Returns:
        None
    """
    with tempfile.TemporaryDirectory() as temp_dir:
        generate_model_package_folder(
            model_jar_content,
            temp_dir,
        )

        cmd = ["jar", "cfvM", model_jar_path, "-C", temp_dir, "."]
        _, err, _ = execute_cmd(cmd)
        if err:
            raise RuntimeError(
                f"Failed to create model jar. Error: {decode_output(err)}",
            )
