import sys
import argparse


def main(args=None):
    p = argparse.ArgumentParser(description="Michelangelo CLI")
    p.add_argument("target", choices=["sandbox"])
    ns = p.parse_args(args=args)

    print(ns)


if __name__ == "__main__":
    sys.exit(main())
