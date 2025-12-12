"""Serialize the main module to the target dir."""

import inspect
import logging
import sys
from typing import Optional

from michelangelo.lib.model_manager._private.utils.module_finder import (
    find_dependency_files,
)
from michelangelo.lib.model_manager._private.utils.module_utils import save_module_files

_logger = logging.getLogger(__name__)


def serialize_main_module(
    target_dir: str, include_import_prefixes: Optional[list[str]] = None
):
    """Serialize the main module to the target dir.

    The dependencies of the main module are also saved,
    excluding the third party dependencies
    All of the serialized files retain the original directory structure.

    Args:
        target_dir: the target dir to serialize the main module
        include_import_prefixes (Optional): only serialize the imported
            modules with the given prefixes,
            e.g. ['uber', 'data.michelangelo'] only imports starting
            with 'uber' or 'data.michelangelo' will be saved in the
            model package. If not specified, save all imports
    """
    main_module = sys.modules["__main__"]

    try:
        inspect.getfile(main_module)
    except TypeError:
        msg = (
            "Warning: Cannot serialize the __main__ module because it is not a file. "
            "Most likely, you are running this code "
            "in an interactive shell or Jupyter notebook. "
            "Please remove dependencies on the __main__ module "
            "and add them to the model class instead. "
            "or run this code in a Python script. "
            "Refer to https://engwiki.uberinternal.com/display/TE0MICHELANGELO/Best+Practices+When+Packaging+the+Model#BestPracticesWhenPackagingtheModel-WhenPackagingaModelinJupyterNotebook"
        )
        _logger.warning(msg)
        print(msg)  # Print to stdout to make sure notebook users see this message
        return

    files = find_dependency_files("__main__", prefixes=include_import_prefixes)
    save_module_files(files, target_dir)
