import argparse
import dataclasses
import importlib
import inspect
import json
import logging
import os
import sys
from typing import Any, Optional

import pydantic

LOGGING_FORMAT = "%(asctime)s | %(levelname)+8.8s | %(name)-40.40s | %(message)s"

log = logging.getLogger(__name__)


def dot_path(arg: Any) -> str:
    module = arg.__module__

    if module == "__main__":
        # Resolve the real module path of the __main__ module.
        file = sys.modules[module].__file__
        assert file

        file = os.path.relpath(file, start=os.getcwd())
        file, _ = os.path.splitext(file)
        assert "." not in file

        module = file.replace(os.path.sep, ".")

    return f"{module}.{arg.__name__}"


def log_attributes(
    logger,
    level,
    obj,
    include_system=False,
):
    if not logger.isEnabledFor(level):
        return
    logger.log(level, "%s", dot_path(type(obj)))
    attrs = dir(obj)
    max_len_attr = max(map(len, attrs))
    for attr in attrs:
        if not include_system and attr.startswith("__") and attr.endswith("__"):
            continue
        val = getattr(obj, attr)
        value_format = "%r"
        logger.log(level, f"  %-{max_len_attr}s | {value_format}", attr, val)


def import_attribute(path: str, package=None):
    m, attr = path.rsplit(".", 1)
    m = importlib.import_module(m, package=package)
    attr = getattr(m, attr)
    return attr


def dataclass_dict(v):
    return {f.name: getattr(v, f.name) for f in dataclasses.fields(v)}


def pydantic_dict(v: pydantic.BaseModel):
    return {f: getattr(v, f) for f in v.model_fields.keys()}


class ArgparseEnvironAction(argparse.Action):
    """Custom argparse action to parse environment variables from command line arguments.

    The action expects a list of strings, where each string can either be in the form 'ENV_VAR=value' or simply
    'ENV_VAR'. If the latter, it fetches the value of 'ENV_VAR' from the current environment variables.

    Usage example:

        import argparse
        parser = argparse.ArgumentParser()
        parser.add_argument('--env', action=EnvironAction, nargs='*', default={})
        args = parser.parse_args(['--env', 'FOO=bar', 'HOME'])
        print(args)
    """

    def __call__(self, _parser, namespace, values, _option_string=None):
        assert isinstance(values, list), values
        dest = getattr(namespace, self.dest)
        assert isinstance(dest, dict), dest
        for v in values:
            k, v = self._parse_value(v)
            dest[k] = v

    @staticmethod
    def _parse_value(s: str):
        """Parses a single command line argument to extract the environment variable and its value.
        Parameters:
            s: The command line argument. Expected format: "ENV_VAR=value" or just "ENV_VAR"
        Returns:
            tuple: A tuple containing the environment variable name and its value.

        Raises:
            KeyError: If the argument is not in the form 'ENV_VAR=value' and the environment variable is not found in os.environ.
        """
        if "=" in s:
            env, val = s.split("=", maxsplit=1)
            return env, val
        if s not in os.environ:
            raise KeyError(f"os.environ not found: {s}")
        return s, os.environ[s]


def encode_value_to_json(value, json_encoder: Optional[json.JSONEncoder] = None):
    """Encode a value to a json file.

    Parameters:
        value: the value to encode.
        json_encoder: the encoder to encode the value.
    """
    import tempfile

    encoder_default = json_encoder.default if json_encoder else None
    with tempfile.NamedTemporaryFile(mode="w", delete=False) as tmp_file:
        json.dump(value, tmp_file, default=encoder_default)

    log.info("tmp_file: %s", tmp_file.name)
    return tmp_file.name


def is_dataclass_instance(value):
    return dataclasses.is_dataclass(value) and not inspect.isclass(value)
