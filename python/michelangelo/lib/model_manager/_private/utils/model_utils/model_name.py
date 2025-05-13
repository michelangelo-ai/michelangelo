import os
from michelangelo.lib.model_manager._private.constants import Placeholder


def replace_model_name_placeholder(model_path: str, model_name: str):
    """
    Replace the model name placeholder in the config.pbtxt file of the model package

    Args:
        model_path: the path of the model in local
        model_name: the actual name of the model
    """
    if not os.path.exists(model_path):
        raise FileNotFoundError(f"{model_path} does not exists.")

    for dirpath, _, filenames in os.walk(model_path):
        if "config.pbtxt" in filenames:
            config_path = os.path.join(dirpath, "config.pbtxt")
            with open(config_path, "r+") as f:
                config = f.read()
                config = config.replace(Placeholder.MODEL_NAME, model_name)
                f.seek(0)
                f.write(config)
                f.truncate()
