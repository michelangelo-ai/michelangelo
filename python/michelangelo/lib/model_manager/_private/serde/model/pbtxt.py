"""Find fields in a Triton ``config.pbtxt`` without a protobuf dependency.

Only what the loaders need is read: a top-level string field, a top-level list
of messages, and the scalar fields of one message. Text inside quoted strings
and ``#`` comments is never matched, and fields can come in any order or on
one line.
"""

from __future__ import annotations

import re
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Iterator

_FIELD_NAME = re.compile(r"[A-Za-z_]\w*")
_STRING_VALUE = re.compile(r'\s*:\s*"([^"]*)"')
_LIST_START = re.compile(r"\s*:?\s*\[")
_MESSAGE_FIELD = re.compile(
    r'(?<![\w."])(\w+)\s*:\s*("(?:[^"\\]|\\.)*"|\[[^\]]*\]|[^\s,{}\[\]]+)'
)


def _string_end(content: str, start: int) -> int:
    """Return the index of the quote that closes the string opened at ``start``."""
    i = start + 1
    while i < len(content):
        if content[i] == "\\":
            i += 2
            continue
        if content[i] == '"':
            return i
        i += 1
    return len(content) - 1


def _top_level_fields(content: str) -> Iterator[tuple[str, int]]:
    """Yield ``(name, end)`` for each field name at nesting depth 0."""
    depth = 0
    i = 0
    while i < len(content):
        char = content[i]
        if char == '"':
            i = _string_end(content, i) + 1
            continue
        if char == "#":
            end = content.find("\n", i)
            i = len(content) if end == -1 else end + 1
            continue
        if char in "{[":
            depth += 1
        elif char in "}]":
            depth -= 1
        elif depth == 0:
            match = _FIELD_NAME.match(content, i)
            if match:
                yield match.group(), match.end()
                i = match.end()
                continue
        i += 1


def _matching_close(content: str, open_index: int) -> int:
    """Return the index of the bracket that closes the one at ``open_index``."""
    depth = 0
    i = open_index
    while i < len(content):
        char = content[i]
        if char == '"':
            i = _string_end(content, i) + 1
            continue
        if char in "{[":
            depth += 1
        elif char in "}]":
            depth -= 1
            if depth == 0:
                return i
        i += 1
    return len(content)


def find_string_field(content: str, name: str) -> str | None:
    """Return the value of the top-level string field ``name``, such as ``backend``.

    Args:
        content: The ``config.pbtxt`` text.
        name: The field name.

    Returns:
        The unquoted value, or ``None`` if the field is not set.
    """
    for field, end in _top_level_fields(content):
        if field == name:
            match = _STRING_VALUE.match(content, end)
            if match:
                return match.group(1)
    return None


def find_messages(content: str, name: str) -> list[str]:
    """Return the messages of the top-level list ``name``, such as ``input``.

    Both ``input [ ... ]`` and ``input: [ ... ]`` are accepted.

    Args:
        content: The ``config.pbtxt`` text.
        name: The field name.

    Returns:
        The text inside each ``{ ... }`` message of the list, in order. Empty if
        the field is not set.
    """
    for field, end in _top_level_fields(content):
        if field != name:
            continue
        match = _LIST_START.match(content, end)
        if not match:
            continue
        list_end = _matching_close(content, match.end() - 1)
        messages = []
        i = match.end()
        while i < list_end:
            if content[i] == "{":
                close = _matching_close(content, i)
                messages.append(content[i + 1 : close])
                i = close
            i += 1
        return messages
    return []


def message_fields(message: str) -> dict[str, str]:
    """Return the scalar and list fields set directly in one message.

    Nested messages, such as ``reshape: { shape: [ 1 ] }``, are skipped.

    Args:
        message: The text inside a ``{ ... }`` message.

    Returns:
        The raw value text of each field, keyed by name. Quoted strings are
        unquoted; lists keep their brackets.
    """
    fields = {}
    depth = 0
    flat = []
    i = 0
    while i < len(message):
        char = message[i]
        if char == '"':
            end = _string_end(message, i)
            if depth == 0:
                flat.append(message[i : end + 1])
            i = end + 1
            continue
        # A nested message is kept as "{}" so its name never takes the next
        # field as its value.
        if char == "{":
            depth += 1
            if depth == 1:
                flat.append(char)
        elif char == "}":
            depth -= 1
            if depth == 0:
                flat.append(char)
        elif depth == 0:
            flat.append(char)
        i += 1
    for name, value in _MESSAGE_FIELD.findall("".join(flat)):
        fields.setdefault(name, value.strip('"'))
    return fields
