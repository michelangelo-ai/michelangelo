"""Generic pickle/``BytesIO`` encode-decode helpers for workflow variable metadata.

Model/feature-package metadata carries some fields as a serialised
``BytesIO`` payload (see ``ModelMetadata``) rather than a live object, since a
live object passed by value can be large enough to exceed a workflow
orchestrator's task-argument-size limits. These two functions are the single
place that pickling/unpickling logic lives, so metadata classes don't each
reimplement their own ``seek(0)`` / ``pickle.loads`` / ``pickle.dumps``.
"""

from __future__ import annotations

import pickle
from io import BytesIO
from typing import TypeVar

T = TypeVar("T")


def retrieve_object(payload: T | BytesIO | None) -> T | None:
    """Decode a value from its serialised ``BytesIO`` form.

    Args:
        payload: A pickled ``BytesIO`` payload, an already-decoded object, or
            ``None``.

    Returns:
        ``None`` if ``payload`` is ``None``; the unpickled object if
        ``payload`` is a ``BytesIO``; ``payload`` itself otherwise.
    """
    if payload is None:
        return None
    if isinstance(payload, BytesIO):
        payload.seek(0)
        return pickle.load(payload)
    return payload


def save_object(value: T | BytesIO | None) -> BytesIO | None:
    """Encode a value into its serialised ``BytesIO`` form.

    Args:
        value: The object to serialise, an already-serialised ``BytesIO``, or
            ``None``.

    Returns:
        ``None`` if ``value`` is ``None``; ``value`` itself if already a
        ``BytesIO``; otherwise a new ``BytesIO`` containing the pickled value.
    """
    if value is None or isinstance(value, BytesIO):
        return value
    return BytesIO(pickle.dumps(value))
