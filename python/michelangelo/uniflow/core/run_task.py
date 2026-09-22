#!/usr/bin/env python3
"""Entrypoint for running a Uniflow task."""

import argparse
import logging
import sys

import fsspec

from michelangelo.uniflow.core.codec import decoder
from michelangelo.uniflow.core.utils import LOGGING_FORMAT, import_attribute

log = logging.getLogger(__name__)


def main():
    """Entrypoint for running a Uniflow task."""
    for a in sys.argv:
        log.info("sys.argv: %s", a)

    p = argparse.ArgumentParser()
    p.add_argument("--task", required=True, type=str)
    p.add_argument("--args", required=True, type=str)
    kwargs_group = p.add_mutually_exclusive_group(required=True)
    kwargs_group.add_argument("--kwargs", type=str)
    kwargs_group.add_argument("--kwargs-file", type=str)
    p.add_argument("--result-url", required=True, type=str)
    p.add_argument("--overrides", type=str)
    ns = p.parse_args()

    args = _decode_arg(ns.args)
    kwargs = (
        _decode_arg(ns.kwargs)
        if ns.kwargs is not None
        else _decode_kwargs_file(ns.kwargs_file)
    )
    overrides = _decode_arg(ns.overrides) if ns.overrides is not None else None

    assert isinstance(args, list), f"Expected args to be a list, but got {type(args)}"
    assert isinstance(kwargs, dict), (
        f"Expected kwargs to be a dict, but got {type(kwargs)}"
    )
    assert isinstance(ns.result_url, str), (
        f"Expected result_url to be a string, but got {type(ns.result_url)}"
    )
    assert ns.result_url.endswith(".json"), (
        f"Expected result_url to end with .json, but got {ns.result_url}"
    )

    task = import_attribute(ns.task)

    assert type(task).__name__ == "TaskFunction", (
        f"Expected task to be a TaskFunction instance, but got instance of {type(task)}"
    )

    if overrides:
        assert isinstance(overrides, dict)
        task = task.with_overrides(**overrides)

    task(
        *args,
        **kwargs,
        _uf_result_url=ns.result_url,
    )
    log.info("[ ok ]")


def _decode_arg(value: str):
    """A type conversion function for argparse arguments that use Uniflow decoder.

    Wraps `decoder.decode` to ensure stack trace logging on failure.
    This avoids argparse's default behavior of suppressing traceback printing in type
    conversion functions, making decoding errors easier to debug.

    See: https://github.com/michelangelo-ai/michelangelo/issues/699
    """
    try:
        return decoder.decode(value)
    except Exception as e:
        error_message = f"Failed to decode argument: {value}"
        log.error(error_message, exc_info=True)
        raise argparse.ArgumentTypeError(error_message) from e


def _decode_kwargs_file(path: str):
    """Read and decode task keyword arguments from an fsspec URL or path."""
    try:
        with fsspec.open(path, mode="rt", encoding="utf-8") as stream:
            return decoder.decode(stream.read())
    except Exception as e:
        error_message = f"Failed to decode kwargs file: {path}"
        log.error(error_message, exc_info=True)
        raise argparse.ArgumentTypeError(error_message) from e


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format=LOGGING_FORMAT)
    sys.exit(main())
