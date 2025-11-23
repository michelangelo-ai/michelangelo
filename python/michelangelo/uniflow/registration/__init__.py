"""
Michelangelo Uniflow Registration Module

This module provides utilities for building, uploading, and registering
Uniflow pipelines with Michelangelo pipeline services.

The module supports both direct registration (for development/testing) and
subprocess-based registration (for production MaCTL usage with environment isolation).
"""

# Subprocess module is available for import but not exposed in __all__
# to maintain clean API surface while allowing MaCTL to access it
from michelangelo.uniflow.registration import subprocess
from michelangelo.uniflow.registration.config_builder import (
    ConfigBuilder,
    ConfigEncoder,
)
from michelangelo.uniflow.registration.register import (
    main,
    prepare_uniflow_input,
    register,
    register_pipeline,
)
from michelangelo.uniflow.registration.uniflow_tar import (
    UniflowTarBuilder,
    prepare_uniflow_tar,
)

__all__ = [
    "register",
    "register_pipeline",
    "main",
    "prepare_uniflow_input",
    "UniflowTarBuilder",
    "prepare_uniflow_tar",
    "ConfigBuilder",
    "ConfigEncoder",
    "subprocess",
]
