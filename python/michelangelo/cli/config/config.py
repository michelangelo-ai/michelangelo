import sys
import argparse

description = "Config CLI"


def init_arguments(p: argparse.ArgumentParser):
    sp = p.add_subparsers(dest="action", required=True)

    _ = sp.add_parser("current-context")
    _ = sp.add_parser("get-contexts")


def main(args=None):
    p = argparse.ArgumentParser(description=description)
    init_arguments(p)
    ns = p.parse_args(args=args)
    return run(ns)


def run(ns: argparse.Namespace):
    if ns.action == "current-context":
        return _current_context(ns)
    if ns.action == "get-contexts":
        return _get_contexts(ns)

    raise ValueError(f"Unsupported action: {ns.action}")


def _current_context(ns: argparse.Namespace):
    assert ns
    print("_current_context")


def _get_contexts(ns: argparse.Namespace):
    assert ns
    print("_get_contexts")


if __name__ == "__main__":
    sys.exit(main())
