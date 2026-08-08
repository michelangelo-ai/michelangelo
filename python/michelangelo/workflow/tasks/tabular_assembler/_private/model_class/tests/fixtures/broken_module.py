"""Fixture module whose top-level code raises an exception on import.

Used to prove ``try_load_class`` only degrades to ``None`` for its
documented exception types; anything else raised by the imported module's
own top-level code (e.g. a plain ``RuntimeError``) propagates uncaught.
"""

raise RuntimeError("boom: this module is broken on purpose")
