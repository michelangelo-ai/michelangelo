"""Shared serialization helpers used by multiple backend packagers."""

import os
import shutil
from typing import Optional, Union

import michelangelo.lib.model_manager.interface.custom_model as custom_model
from michelangelo.lib.model_manager._private.packager.template_renderer import (
    TritonTemplateRenderer,
)
from michelangelo.lib.model_manager._private.utils.module_finder import (
    find_dependency_files,
)
from michelangelo.lib.model_manager._private.utils.module_utils import save_module_files

_INTERFACE_MODULE_PATH = os.path.join(
    "michelangelo", "lib", "model_manager", "interface", "custom_model.py"
)


def serialize_model_interface(target_dir: str) -> None:
    """Serialize the custom_model interface into the target dir.

    Args:
        target_dir: the target dir to serialize the model interface
    """
    target_path = os.path.join(target_dir, _INTERFACE_MODULE_PATH)
    if not os.path.exists(target_path):
        os.makedirs(os.path.dirname(target_path), exist_ok=True)
        shutil.copyfile(custom_model.__file__, target_path)


def serialize_model_class(
    model_class: str,
    target_dir: str,
    model_file_name: str,
    include_import_prefixes: Optional[list[str]] = None,
    serialize_interface: bool = True,
    write_txt_file: bool = True,
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
            e.g. ['mypackage', 'data.myproject'] only imports starting
            with those prefixes will be saved in the
            model package. If not specified, save all imports
        serialize_interface: if True (default), serialize the model
            interface into the target dir. If False, skip interface
            serialization.
        write_txt_file: if True (default), write the text file containing
            the import path of the model class. If False, skip writing it.

    Returns:
        None
    """
    os.makedirs(target_dir, exist_ok=True)

    module_def, _, _ = model_class.rpartition(".")

    # serialize the model class along with its dependencies
    # all of the serialized files retain the original directory structure
    files = find_dependency_files(module_def, prefixes=include_import_prefixes)
    save_module_files(files, target_dir)

    # create the model class file
    if write_txt_file:
        with open(os.path.join(target_dir, model_file_name), "w") as f:
            f.write(model_class)

    # serialize the model interface
    if serialize_interface:
        serialize_model_interface(target_dir)


def generate_model_py_content(
    gen: TritonTemplateRenderer,
) -> str:
    """Generate the model.py file content.

    Args:
        gen: The TritonTemplateRenderer instance

    Returns:
        The model.py file content
    """
    return gen.render("model.py.tmpl")


def generate_requirements_txt(requirements: Union[list[str], str]) -> str:
    """Generate the requirements.txt file content.

    Args:
        requirements: The requirements can be one of the following:
            - A string representing the requirements.txt file path
            - A list of strings representing the requirements, e.g
              ["numpy==1.18.5", "pandas==1.0.5"]

    Returns:
        The requirements.txt file content
    """
    if isinstance(requirements, str):
        with open(requirements) as f:
            return f.read()

    if isinstance(requirements, list):
        return "\n".join([str(r) for r in requirements])

    raise ValueError(
        "requirements must be a list of requirements or "
        f"the requirements.txt file path, but got {type(requirements).__name__}"
    )
