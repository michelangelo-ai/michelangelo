import sys
import argparse
import logging
from michelangelo.cli.sandbox import sandbox

description = """
The Michelangelo CLI is a unified tool that provides access to Michelangelo services through the terminal.
"""


def main(args=None):
    logging.basicConfig(level=logging.INFO)

    p = argparse.ArgumentParser(description=description)
    sp = p.add_subparsers(dest="entity", required=True, help="Entity to operate on")

    sandbox_p = sp.add_parser(
        "sandbox",
        description=sandbox.description,
        help=sandbox.short_description,
    )
    sandbox.init_arguments(sandbox_p)

    ns = p.parse_args(args=args)

    if ns.entity == "sandbox":
        return sandbox.run(ns)

    raise ValueError("Unsupported entity: %s" % ns.entity)


if __name__ == "__main__":
    sys.exit(main())
