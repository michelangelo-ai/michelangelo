"""Michelangelo package init — declares a shared-namespace package.

`michelangelo` is a shared-namespace package: contents from multiple
``sys.path`` entries merge into one logical package. The legacy
:func:`pkgutil.extend_path` form is used (rather than PEP 420) to keep
``__file__`` defined and to remain compatible with downstream consumers
that previously treated this as a regular package.

Bazel / PEP consumers that bundle the wheel alongside separately-generated
proto stubs (e.g. from local IDL trees) need cross-``sys.path`` merging to
import ``michelangelo.api.*``.
"""

__path__ = __import__("pkgutil").extend_path(__path__, __name__)

import michelangelo._nightly_warning  # noqa: F401
