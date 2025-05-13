import logging
from typing import Optional
from uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils import (
    find_pickled_files,
    find_pickle_definitions,
)
from uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder import find_dependency_files
from uber.ai.michelangelo.sdk.model_manager._private.utils.module_utils import save_module_files
from uber.ai.michelangelo.sdk.model_manager._private.packager.python_triton.main_module import serialize_main_module

_logger = logging.getLogger(__name__)


def serialize_pickle_dependencies(model_path: str, target_dir: str, include_import_prefixes: Optional[list[str]] = None):
    """
    Serialize dependencies of all pickled files to target directory.

    Args:
        model_path (str): The path to the model.
        target_dir (str): The target directory to serialize the dependencies.
        include_import_prefixes (Optional[list[str]]): Only save the imported
            modules with the given prefixes in the model package,
            e.g. ['uber', 'data.michelangelo'] only imports starting
            with 'uber' or 'data.michelangelo' will be saved in the
            model package. Default is ['uber'],
            and if the list is empty, save all imports.
    """
    need_main_module = False

    pickled_files = find_pickled_files(model_path)
    for pickled_file in pickled_files:
        _logger.info(f"Serializing dependencies of the pickled file {pickled_file} to {target_dir}")
        pickle_definitions = find_pickle_definitions(pickled_file)
        if not need_main_module and any(definition.rpartition(".")[0] == "__main__" for definition in pickle_definitions):
            need_main_module = True
        serialize_pickle_definitions(pickle_definitions, target_dir, include_import_prefixes)

    if need_main_module:
        _logger.info(f"Serializing the __main__ module to {target_dir}")
        serialize_main_module(target_dir, include_import_prefixes)


def serialize_pickle_definitions(pickle_definitions: list[str], target_dir: str, include_import_prefixes: Optional[list[str]] = None):
    """
    Serialize dependencies of a list of pickle definitions to target directory.

    Args:
        pickle_definitions (list[str]): The list of pickle definitions.
        target_dir (str): The target directory to serialize the dependencies.
        include_import_prefixes (Optional[list[str]]): Only save the imported
            modules with the given prefixes in the model package,
            e.g. ['uber', 'data.michelangelo'] only imports starting
            with 'uber' or 'data.michelangelo' will be saved in the
            model package. Default is ['uber'],
            and if the list is empty, save all imports.
    """
    for definition in pickle_definitions:
        module_def, _, _ = definition.rpartition(".")

        if include_import_prefixes and not any(module_def.startswith(prefix) for prefix in include_import_prefixes):
            continue

        files = find_dependency_files(module_def, prefixes=include_import_prefixes)
        save_module_files(files, target_dir)


def serialize_pickled_file_dependencies(pickled_file: str, target_dir: str, include_import_prefixes: Optional[list[str]] = None):
    """
    Serialize dependencies of a pickled file to target directory.

    Args:
        pickled_file (str): The path to the pickled file.
        target_dir (str): The target directory to serialize the dependencies.
        include_import_prefixes (Optional[list[str]]): Only save the imported
            modules with the given prefixes in the model package,
            e.g. ['uber', 'data.michelangelo'] only imports starting
            with 'uber' or 'data.michelangelo' will be saved in the
            model package. Default is ['uber'],
            and if the list is empty, save all imports.
    """
    pickle_definitions = find_pickle_definitions(pickled_file)
    serialize_pickle_definitions(pickle_definitions, target_dir, include_import_prefixes)
