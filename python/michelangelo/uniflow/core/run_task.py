#!/usr/bin/env python3
import argparse
import logging
import sys

from michelangelo.uniflow.core.codec import decoder
from michelangelo.uniflow.core.utils import LOGGING_FORMAT, import_attribute

log = logging.getLogger(__name__)


def main():
    """Entrypoint for running a Uniflow task."""
    for a in sys.argv:
        log.info("sys.argv: %s", a)

    p = argparse.ArgumentParser()
    p.add_argument("--task", required=True, type=str)
    p.add_argument("--args", required=True, type=_decode_arg)
    p.add_argument("--kwargs", required=True, type=_decode_arg)
    p.add_argument("--result-url", required=True, type=str)
    p.add_argument("--overrides", type=_decode_arg)
    ns = p.parse_args()

    assert isinstance(ns.args, list), (
        f"Expected args to be a list, but got {type(ns.args)}"
    )
    assert isinstance(ns.kwargs, dict), (
        f"Expected kwargs to be a dict, but got {type(ns.kwargs)}"
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

    if ns.overrides:
        assert isinstance(ns.overrides, dict)
        task = task.with_overrides(**ns.overrides)

    task(
        *ns.args,
        **ns.kwargs,
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


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format=LOGGING_FORMAT)
    sys.exit(main())
