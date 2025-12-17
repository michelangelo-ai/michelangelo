"""Utilities for the simple_custom_torch example (top-level module under lib/)."""

from __future__ import annotations

import os

import torch

from examples.model_manager.simple_custom_torch.lib.constants import STATE_DICT_FILENAME
from examples.model_manager.simple_custom_torch.lib.regular_pkg.nested import join_parts


def state_dict_path(model_dir: str) -> str:
    # Calls into nested regular package.
    filename = join_parts("", STATE_DICT_FILENAME).lstrip("/")
    return os.path.join(model_dir, filename)


def save_state_dict(model_dir: str, state_dict: dict) -> None:
    os.makedirs(model_dir, exist_ok=True)
    torch.save(state_dict, state_dict_path(model_dir))


def load_state_dict(model_dir: str) -> dict:
    return torch.load(state_dict_path(model_dir), map_location="cpu")
