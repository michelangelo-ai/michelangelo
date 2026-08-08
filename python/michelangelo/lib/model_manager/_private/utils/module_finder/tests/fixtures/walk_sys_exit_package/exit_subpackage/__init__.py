"""Subpackage whose import unconditionally calls ``sys.exit`` -- used to test
that ``find_dependency_files_internal`` survives ``pkgutil.walk_packages()``
re-raising ``SystemExit`` while probing this subpackage for recursion.
"""

import sys

sys.exit(1)
