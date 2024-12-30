import sys
import argparse
import logging
from michelangelo.cli.sandbox import sandbox
from michelangelo.cli.config import config


def main(args=None):
    logging.basicConfig(level=logging.INFO)

    p = argparse.ArgumentParser(description="Michelangelo CLI")
    sp = p.add_subparsers(dest="entity", required=True, help="Entity to operate on")

    sandbox_p = sp.add_parser("sandbox", description=sandbox.description)
    sandbox.init_arguments(sandbox_p)

    config_p = sp.add_parser("config", description=config.description)
    config.init_arguments(config_p)

    ns = p.parse_args(args=args)

    if ns.entity == "sandbox":
        return sandbox.run(ns)

    if ns.entity == "config":
        return config.run(ns)

    raise ValueError("Unsupported entity: %s" % ns.entity)


if __name__ == "__main__":
    sys.exit(main())
