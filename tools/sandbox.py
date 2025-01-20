#!/usr/bin/env python3

import sys
import utils

utils.include_python_dir()

from michelangelo.cli.sandbox.sandbox import main

if __name__ == "__main__":
    sys.exit(main())
