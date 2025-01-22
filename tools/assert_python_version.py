#!/usr/bin/env python3

import sys

"""
This CLI checks if the current Python version meets the required version.

Usage:
    ./tools/assert_python_version.py <required_version>

Example:
    ./tools/assert_python_version.py 3.9
"""


def main():

    try:
        version = sys.argv[1]
        version_info = tuple(map(int, version.split(".")))
    except ValueError:
        print(
            "Invalid version format. Please use a format like '3.9'.",
            file=sys.stderr,
        )
        sys.exit(1)

    if sys.version_info < version_info:
        print(
            f"ERROR: Python {version} or higher is required. You are using Python {sys.version}.",
            file=sys.stderr,
        )
        sys.exit(1)


if __name__ == "__main__":
    main()
