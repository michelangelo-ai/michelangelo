import itertools
import sys
import traceback

from michelangelo.lib.model_manager._private.utils.pickle_utils import (
    walk_pickle_definitions_in_dir,
)
from michelangelo.lib.model_manager._private.utils.reflection_utils import (
    find_attr_from_dir,
)
from michelangelo.lib.model_manager.interface.custom_model import Model


def load_custom_model(model_bin_path: str, ModelClass: type, defs_path: str) -> Model:
    """Load the custom model binary into an instance of the custom model class (ModelClass).

    Args:
        model_bin_path: The path to the model binary.
        ModelClass: The custom model class, must be a subclass of Model.
        defs_path: The path to the directory containing Python files defining the required attributes.

    Returns:
        The loaded custom model instance.
    """
    main_module = sys.modules["__main__"]
    original_attr_dict = main_module.__dict__.copy()

    def match(
        module_def: str,
        attr_name: str,
        file_path: str,
    ) -> bool:
        return module_def == "__main__"

    # Search for all possible combinations of attributes for the list of attribute names under __main__ required
    # by the pickled files (if there are any) and attempt to load the model with each combination of attributes
    # Note: This approach has exponential time complexity in the worst case scenario. However, it is unlikely
    # that the user defines many different class/functions with the same name as the one in __main__, and the
    # number of attributes needed in __main__ is usually small. The average time complexity of this approach
    # should approach O(n) where n is the number of the required attributes in __main__.
    attr_names = [
        attr_name
        for _, attr_name, _ in walk_pickle_definitions_in_dir(
            model_bin_path, match=match
        )
    ]
    attr_lists = [
        [
            *(
                [main_module.__dict__[attr_name]]
                if attr_name in main_module.__dict__
                else []
            ),
            *find_attr_from_dir(attr_name, defs_path),
        ]
        for attr_name in attr_names
    ]

    for attr_combination in itertools.product(*attr_lists):
        if not attr_combination:
            continue

        for attr_name, attr in zip(attr_names, attr_combination):
            main_module.__dict__[attr_name] = attr

        try:
            model = ModelClass.load(model_bin_path)
        except AttributeError:
            continue
        else:
            return model

    # Load the model directly if there is no pickle files requiring attributes from __main__
    # or load again to throw the original exception
    try:
        return ModelClass.load(model_bin_path)
    except Exception as err:
        # clean up the added attributes in __main__
        for attr_name in attr_names:
            if attr_name in main_module.__dict__:
                if attr_name in original_attr_dict:
                    main_module.__dict__[attr_name] = original_attr_dict[attr_name]
                else:
                    del main_module.__dict__[attr_name]

        err_msg = "".join(traceback.format_exception(type(err), err, err.__traceback__))
        raise RuntimeError(f"Unable to load the model:\nError {err_msg}") from err
