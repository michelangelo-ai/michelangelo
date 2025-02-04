#!/usr/bin/env python3
import argparse
import sys
import logging
from michelangelo.uniflow.core.codec import decoder
from michelangelo.uniflow.core.decorator import TaskFunction
from michelangelo.uniflow.core.utils import LOGGING_FORMAT, import_attribute

log = logging.getLogger(__name__)


def main():
    for a in sys.argv:
        log.info("sys.argv: %s", a)

    p = argparse.ArgumentParser()
    p.add_argument("--task", required=True, type=import_attribute)
    p.add_argument("--args", required=True, type=decoder.decode)
    p.add_argument("--kwargs", required=True, type=decoder.decode)
    p.add_argument("--result-url", required=True, type=str)
    p.add_argument("--overrides", type=decoder.decode)
    ns = p.parse_args()

    assert isinstance(ns.task, TaskFunction)
    assert isinstance(ns.args, list)
    assert isinstance(ns.kwargs, dict)
    assert isinstance(ns.result_url, str)
    assert ns.result_url.endswith(".json")

    task = ns.task
    if ns.overrides:
        assert isinstance(ns.overrides, dict)
        task = task.with_overrides(**ns.overrides)

    task(
        *ns.args,
        **ns.kwargs,
        _uf_result_url=ns.result_url,
    )
    log.info("[ ok ]")


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format=LOGGING_FORMAT)
    sys.exit(main())
