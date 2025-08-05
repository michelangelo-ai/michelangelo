"""
Michelangelo Uniflow Registration Module

This module provides utilities for building, uploading, and registering
Uniflow pipelines with Michelangelo pipeline services.

The module supports both direct registration (for development/testing) and
subprocess-based registration (for production MaCTL usage with environment isolation).
"""

from michelangelo.uniflow.registration.register import (
    register,
    main,
    prepare_uniflow_input,
    register_pipeline,
)
from michelangelo.uniflow.registration.uniflow_tar import (
    UniflowTarBuilder,
    prepare_uniflow_tar,
)
from michelangelo.uniflow.registration.config_builder import ConfigBuilder, ConfigEncoder
from michelangelo.uniflow.registration.external_storage import ExternalStorageHandler, default_external_storage

# Subprocess module is available for import but not exposed in __all__
# to maintain clean API surface while allowing MaCTL to access it
from michelangelo.uniflow.registration import subprocess

__all__ = [
    "register",
    "register_pipeline",
    "main",
    "prepare_uniflow_input",
    "UniflowTarBuilder",
    "prepare_uniflow_tar",
    "ConfigBuilder",
    "ConfigEncoder",
    "ExternalStorageHandler",
    "default_external_storage",
    "subprocess",
]
