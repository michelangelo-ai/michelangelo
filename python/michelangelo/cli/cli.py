"""Entry point for the ``ma`` command-line interface."""

import argparse
import logging
import sys

from michelangelo.cli.importer import importer
from michelangelo.cli.mactl import mactl
from michelangelo.cli.sandbox import sandbox

description = """
Michelangelo CLI is a command-line interface that enables seamless access to
Michelangelo services directly from your terminal.
"""


def main(args=None):
    """Dispatch to the ``ma`` subcommand named by ``args`` or ``sys.argv``."""
    logging.basicConfig(level=logging.INFO)

    # Determine entity from args or sys.argv
    if args is not None and len(args) > 0:
        entity = args[0]
    elif len(sys.argv) > 1:
        entity = sys.argv[1]
    else:
        entity = None

    if entity == "sandbox":
        p = argparse.ArgumentParser(description=description)
        sp = p.add_subparsers(dest="entity", required=True, help="Entity to operate on")
        sandbox_p = sp.add_parser(
            "sandbox",
            description=sandbox.description,
            help=sandbox.short_description,
        )
        sandbox.init_arguments(sandbox_p)
        ns = p.parse_args(args=args)
        return sandbox.run(ns)

    if entity == "import":
        p = argparse.ArgumentParser(description=description)
        sp = p.add_subparsers(dest="entity", required=True, help="Entity to operate on")
        import_p = sp.add_parser(
            "import",
            description=importer.description,
            help=importer.short_description,
        )
        importer.init_arguments(import_p)
        ns = p.parse_args(args=args)
        return importer.run(ns)

    # For all other entities, delegate to mactl
    return mactl.run()


if __name__ == "__main__":
    sys.exit(main())
