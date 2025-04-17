import logging
from abc import ABC, abstractmethod
from io import BytesIO
from typing import Callable, Union, Optional, Any
from typing import TypeVar, Generic

import fsspec

log = logging.getLogger(__name__)

T = TypeVar("T")


class IO(ABC, Generic[T]):
    """
    Base class for IO Plugin.
    """

    @abstractmethod
    def write(self, url: str, value: T) -> Optional[Any]:
        """
        Serializes the given value and saves it to the specified URL.
        Implementation classes should support URLs pointing to remote object stores
        (e.g., gcs://, s3://, hdfs://) as well as local disk paths (e.g., file:// or Unix paths like /home/user/data).

        It is recommended to use file system abstraction libraries (such as fsspec or py-arrow)
        to properly handle IO for the given URL.

        Implementation classes may optionally return metadata about the written data.
        This metadata must not contain the actual value and must be reasonably small. It can be any JSON-serializable
        data structure supported by the available codecs (see codec.py)

        The returned metadata will be passed to the `read` method to facilitate deserialization logic
        for the serialized value.

        Arguments:
            url: The URL where the value should be saved.
            value: The value to be serialized and saved.

        Returns:
            Any: Metadata about the written data, or None if no metadata is needed for the `read` logic.
        """
        raise NotImplementedError

    @abstractmethod
    def read(self, url: str, metadata: Optional[Any]) -> T:
        """
        Deserializes and loads an object from the specified URL.
        Implementation classes should support URLs pointing to remote object stores
        (e.g., gcs://, s3://, hdfs://) as well as local disk paths (e.g., file:// or Unix paths like /home/user/data).

        It is recommended to use file system abstraction libraries (such as fsspec or py-arrow)
        to properly handle IO for the given URL.

        Arguments:
            url: The URL from where the object should be loaded.
            metadata: Optional metadata to facilitate the value deserialization and loading logic.

        Returns:
            T: The loaded value.
        """
        raise NotImplementedError


class BytesIOIO(IO[BytesIO]):
    def write(self, url: str, value: BytesIO) -> Optional[Any]:
        with fsspec.open(url, mode="wb") as f:
            f.write(value.getbuffer())
        return None

    def read(self, url: str, _metadata) -> BytesIO:
        with fsspec.open(url, mode="rb") as f:
            return BytesIO(f.read())


LazyIO = Union[IO, Callable[[], IO]]


class IORegistry:
    def __init__(self, registry: dict[type, LazyIO]):
        self._registry = registry

    def set(self, t: type, io: LazyIO, force: bool = False) -> "IORegistry":
        if not force and t in self._registry:
            raise KeyError(
                "IO already registered! type: %r, io: %r, conflicting io: %r",
                t,
                self._registry[t],
                io,
            )

        self._registry[t] = io
        return self

    def update(self, io_dict: dict[type, LazyIO], force: bool = False) -> "IORegistry":
        for t, io in io_dict.items():
            self.set(t, io, force=force)
        return self

    def copy(self) -> "IORegistry":
        return IORegistry(self._registry.copy())

    def __repr__(self):
        return self._registry.__repr__()

    def __getitem__(self, _type: type) -> IO:
        for t in _type.__mro__:
            if t not in self._registry:
                continue
            io = self._registry[t]
            if not isinstance(io, IO):
                assert callable(io)
                io = io()
                assert isinstance(io, IO)
                self._registry[t] = io
            return io

        raise KeyError(f"io not found: type={_type}")

    def __setitem__(self, _type: type, io: LazyIO):
        self.set(_type, io)

    def __contains__(self, _type: type) -> bool:
        for t in _type.__mro__:
            if t in self._registry:
                return True
        return False


# Default IO registry
default_io = IORegistry(
    {
        BytesIO: BytesIOIO,
    }
)


# Deprecated, use default_io instead
def io_registry() -> IORegistry:
    return default_io
