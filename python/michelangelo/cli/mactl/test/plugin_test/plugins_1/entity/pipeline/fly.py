"""Pipeline `fly` function plugin module.

This is a unittest function only for testing purposes.
"""

from inspect import Parameter

from michelangelo.cli.mactl.crd import (
    CRD,
    inject_func_signature,
)


def add_fly_function_signature(crd: CRD) -> None:
    """Add function signature for pipeline kill command."""
    inject_func_signature(
        crd,
        "fly",
        {
            "help": "Fly away all pipelines.",
            "args": [
                {
                    "func_signature": Parameter(
                        "namespace",
                        Parameter.POSITIONAL_OR_KEYWORD,
                    ),
                    "args": ["-n", "--namespace"],
                    "kwargs": {
                        "type": str,
                        "required": True,
                        "help": "Namespace of the resource",
                    },
                },
            ],
        },
    )
