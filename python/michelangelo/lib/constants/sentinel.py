"""Type-native sentinel values used to fill padded/missing numpy positions."""

from __future__ import annotations

INT32_SENTINEL = -(2**31)  # -2147483648, np.iinfo(np.int32).min
FLOAT_SENTINEL = float("nan")
STRING_SENTINEL = ""
BYTES_SENTINEL = b""
BOOL_SENTINEL = False
