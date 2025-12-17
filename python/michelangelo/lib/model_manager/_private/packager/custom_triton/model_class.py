"""Serialize the model class to the target dir."""

import os
from typing import Optional

from michelangelo.lib.model_manager._private.packager.custom_triton.model_interface import (  # noqa: E501
    serialize_model_interface,
)
from michelangelo.lib.model_manager._private.utils.module_finder import (
    find_dependency_files,
)
from michelangelo.lib.model_manager._private.utils.module_utils import save_module_files


def serialize_model_class(
    model_class: str,
    target_dir: str,
    model_file_name: str,
    include_import_prefixes: Optional[list[str]] = None,
):
    """Serialize the model class to the target dir.

    The dependencies of the model class are also saved,
    excluding the third party dependencies
    All of the serialized files retain the original directory structure.
    An additional text file is created in the target dir, which
    contains the import path of the model class.

    Args:
        model_class: the model class
        target_dir: the target dir to serialize the model class
        model_file_name: the name of the model file, which
            is the text file containing the import path of
            the model class
        include_import_prefixes (Optional): only serialize the imported
            modules with the given prefixes,
            e.g. ['uber', 'data.michelangelo'] only imports starting
            with 'uber' or 'data.michelangelo' will be saved in the
            model package. If not specified, save all imports

    Returns:
        None
    """
    os.makedirs(target_dir, exist_ok=True)

    module_def, _, _ = model_class.rpartition(".")

    # serialize the model class along with its dependencies
    # all of the serialized files retain the original directory structure
    files = find_dependency_files(module_def, prefixes=include_import_prefixes)

    print("-------------files-----------------")
    for file in files:
        print(file)
    print("--------------------------------")
    save_module_files(files, target_dir)

    # create the model class file
    with open(os.path.join(target_dir, model_file_name), "w") as f:
        f.write(model_class)

    # serialize the model interface
    serialize_model_interface(target_dir)
