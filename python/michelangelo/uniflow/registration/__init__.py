"""
Michelangelo Uniflow Registration Module

This module provides utilities for building, uploading, and registering
Uniflow pipelines with Michelangelo pipeline services.

The module supports both direct registration (for development/testing) and
subprocess-based registration (for production MaCTL usage with environment isolation).
"""

from .register import register, main, prepare_uniflow_input
from .uniflow_tar import UniflowTarBuilder, prepare_uniflow_tar

# Subprocess module is available for import but not exposed in __all__
# to maintain clean API surface while allowing MaCTL to access it
from . import subprocess

__all__ = [
    "register",
    "main", 
    "prepare_uniflow_input",
    "UniflowTarBuilder",
    "prepare_uniflow_tar",
    "subprocess",
]