"""I/O plugin system for serializing and deserializing Python objects.

This module provides a pluggable I/O system for reading and writing Python objects
to various storage backends. The I/O system supports local file systems and remote
object stores (GCS, S3, HDFS, etc.) through fsspec integration.

The core components are:

- IO: Abstract base class defining the serialization/deserialization interface
- IORegistry: Registry mapping Python types to their corresponding I/O handlers
- BytesIOIO: Built-in I/O handler for BytesIO objects

Example:
    Registering a custom I/O handler::

        from michelangelo.uniflow.core.io_registry import IO, default_io
        import pandas as pd

        class PandasIO(IO[pd.DataFrame]):
            def write(self, url: str, value: pd.DataFrame):
                value.to_parquet(url)
                return None

            def read(self, url: str, metadata):
                return pd.read_parquet(url)

        # Register the handler
        default_io.set(pd.DataFrame, PandasIO())

    Using the I/O registry::

        import pandas as pd
        from michelangelo.uniflow.core.io_registry import default_io

        df = pd.DataFrame({"a": [1, 2, 3]})
        io_handler = default_io[pd.DataFrame]
        io_handler.write("s3://bucket/data.parquet", df)
        loaded_df = io_handler.read("s3://bucket/data.parquet", None)
"""
import logging
from abc import ABC, abstractmethod
from io import BytesIO
from typing import Any, Callable, Generic, Optional, TypeVar, Union

import fsspec

log = logging.getLogger(__name__)

T = TypeVar("T")


class IO(ABC, Generic[T]):
    """Abstract base class for I/O plugins.

    I/O plugins handle serialization and deserialization of Python objects to and from
    storage backends. Implementations must support both local and remote storage URLs
    through fsspec integration.

    Type Parameters:
        T: The Python type that this I/O handler manages.

    Example:
        Implementing a custom I/O handler::

            class MyDataIO(IO[MyData]):
                def write(self, url: str, value: MyData) -> Optional[Any]:
                    with fsspec.open(url, 'wb') as f:
                        pickle.dump(value, f)
                    return None

                def read(self, url: str, metadata: Optional[Any]) -> MyData:
                    with fsspec.open(url, 'rb') as f:
                        return pickle.load(f)
    """

    @abstractmethod
    def write(self, url: str, value: T) -> Optional[Any]:
        """Serialize and write a value to the specified URL.

        Implementation classes should support URLs pointing to remote object stores
        (e.g., gcs://, s3://, hdfs://) as well as local disk paths (e.g., file://
        or Unix paths like /home/user/data).

        It is recommended to use file system abstraction libraries (such as fsspec
        or pyarrow) to properly handle I/O for the given URL.

        Implementation classes may optionally return metadata about the written data.
        This metadata must not contain the actual value and must be reasonably small.
        It can be any JSON-serializable data structure supported by the available
        codecs (see codec.py).

        The returned metadata will be passed to the `read` method to facilitate
        deserialization logic for the serialized value.

        Args:
            url: The URL where the value should be saved.
            value: The value to be serialized and saved.

        Returns:
            Optional metadata about the written data, or None if no metadata is
            needed for the `read` logic.
        """
        raise NotImplementedError

    @abstractmethod
    def read(self, url: str, metadata: Optional[Any]) -> T:
        """Deserialize and load an object from the specified URL.

        Implementation classes should support URLs pointing to remote object stores
        (e.g., gcs://, s3://, hdfs://) as well as local disk paths (e.g., file://
        or Unix paths like /home/user/data).

        It is recommended to use file system abstraction libraries (such as fsspec
        or pyarrow) to properly handle I/O for the given URL.

        Args:
            url: The URL from where the object should be loaded.
            metadata: Optional metadata to facilitate the value deserialization
                and loading logic.

        Returns:
            The loaded value.
        """
        raise NotImplementedError


class BytesIOIO(IO[BytesIO]):
    """I/O handler for BytesIO objects.

    Provides serialization and deserialization for in-memory binary buffers
    using the BytesIO class. Writes the binary content directly to storage
    and reads it back into a new BytesIO instance.

    Example:
        Writing and reading BytesIO objects::

            from io import BytesIO
            from michelangelo.uniflow.core.io_registry import default_io

            buffer = BytesIO(b"Hello, World!")
            io_handler = default_io[BytesIO]
            io_handler.write("file:///tmp/data.bin", buffer)
            loaded = io_handler.read("file:///tmp/data.bin", None)
    """

    def write(self, url: str, value: BytesIO) -> Optional[Any]:
        """Write BytesIO buffer to the specified URL.

        Args:
            url: The URL where the binary content should be saved.
            value: The BytesIO object containing the binary data to write.

        Returns:
            None (no metadata needed for BytesIO).
        """
        with fsspec.open(url, mode="wb") as f:
            f.write(value.getbuffer())
        return None

    def read(self, url: str, _metadata) -> BytesIO:
        """Read binary content from URL into a BytesIO buffer.

        Args:
            url: The URL from where the binary content should be loaded.
            _metadata: Unused metadata parameter (BytesIO doesn't need metadata).

        Returns:
            A new BytesIO instance containing the loaded binary data.
        """
        with fsspec.open(url, mode="rb") as f:
            return BytesIO(f.read())


LazyIO = Union[IO, Callable[[], IO]]


class IORegistry:
    """Registry mapping Python types to their I/O handlers.

    The IORegistry maintains a mapping from Python types to their corresponding I/O
    implementations. It supports lazy initialization of I/O handlers through callable
    factories and provides inheritance-based lookup through the MRO (Method Resolution
    Order).

    Attributes:
        _registry: Internal dictionary mapping types to I/O handlers or factories.

    Example:
        Creating and using a custom registry::

            from michelangelo.uniflow.core.io_registry import IORegistry, BytesIOIO
            from io import BytesIO

            # Create a new registry
            custom_io = IORegistry({BytesIO: BytesIOIO()})

            # Register additional handlers
            custom_io.set(pd.DataFrame, PandasIO())

            # Lookup and use handlers
            handler = custom_io[BytesIO]
            handler.write("s3://bucket/data.bin", my_bytes)
    """

    def __init__(self, registry: dict[type, LazyIO]):
        """Initialize the I/O registry.

        Args:
            registry: Dictionary mapping Python types to I/O handlers or factories.
                Factories are callables that return I/O instances when invoked.
        """
        self._registry = registry

    def set(self, t: type, io: LazyIO, force: bool = False) -> "IORegistry":
        """Register an I/O handler for a specific type.

        Args:
            t: The Python type to register the handler for.
            io: The I/O handler instance or a factory callable that returns an
                I/O handler.
            force: If True, overwrites existing registrations. If False, raises
                KeyError if the type is already registered. Defaults to False.

        Returns:
            The IORegistry instance (for method chaining).

        Raises:
            KeyError: If the type is already registered and force=False.

        Example:
            Registering a new I/O handler::

                from michelangelo.uniflow.core.io_registry import default_io

                default_io.set(MyType, MyTypeIO())
                # Or use a lazy factory
                default_io.set(MyType, lambda: MyTypeIO())
        """
        if not force and t in self._registry:
            raise KeyError(
                "IO already registered! type: %r, io: %r, conflicting io: %r",
                t,
                self._registry[t],
                io,
            )

        self._registry[t] = io
        return self

    def update(
        self, io_dict: dict[type, LazyIO], force: bool = False
    ) -> "IORegistry":
        """Register multiple I/O handlers at once.

        Args:
            io_dict: Dictionary mapping types to I/O handlers or factories.
            force: If True, overwrites existing registrations. Defaults to False.

        Returns:
            The IORegistry instance (for method chaining).

        Raises:
            KeyError: If any type is already registered and force=False.

        Example:
            Bulk registration::

                default_io.update({
                    pd.DataFrame: PandasIO(),
                    np.ndarray: NumpyIO(),
                })
        """
        for t, io in io_dict.items():
            self.set(t, io, force=force)
        return self

    def copy(self) -> "IORegistry":
        """Create a shallow copy of the registry.

        Returns:
            A new IORegistry instance with a copy of the internal registry dict.

        Example:
            Creating a custom registry from the default::

                custom_io = default_io.copy()
                custom_io.set(MyType, MyTypeIO())
        """
        return IORegistry(self._registry.copy())

    def __repr__(self):
        """Return string representation of the registry.

        Returns:
            String representation showing the internal registry dict.
        """
        return self._registry.__repr__()

    def __getitem__(self, _type: type) -> IO:
        """Retrieve the I/O handler for a given type.

        Performs inheritance-based lookup through the type's MRO. If a lazy factory
        is registered, it will be instantiated and cached.

        Args:
            _type: The Python type to lookup the I/O handler for.

        Returns:
            The I/O handler instance for the given type.

        Raises:
            KeyError: If no I/O handler is registered for the type or any of its
                base classes.

        Example:
            Looking up I/O handlers::

                from io import BytesIO
                from michelangelo.uniflow.core.io_registry import default_io

                handler = default_io[BytesIO]
                handler.write("file:///tmp/data.bin", my_buffer)
        """
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
        """Register an I/O handler using dictionary syntax.

        Args:
            _type: The Python type to register the handler for.
            io: The I/O handler instance or factory callable.

        Raises:
            KeyError: If the type is already registered.

        Example:
            Dictionary-style registration::

                default_io[MyType] = MyTypeIO()
        """
        self.set(_type, io)

    def __contains__(self, _type: type) -> bool:
        """Check if an I/O handler is registered for a type.

        Performs inheritance-based lookup through the type's MRO.

        Args:
            _type: The Python type to check.

        Returns:
            True if a handler is registered for the type or any base class,
            False otherwise.

        Example:
            Checking for registered handlers::

                from io import BytesIO
                from michelangelo.uniflow.core.io_registry import default_io

                if BytesIO in default_io:
                    print("BytesIO handler is available")
        """
        return any(t in self._registry for t in _type.__mro__)


# Default IO registry
default_io = IORegistry(
    {
        BytesIO: BytesIOIO,
    }
)


# Deprecated, use default_io instead
def io_registry() -> IORegistry:
    """Return the default I/O registry.

    .. deprecated::
        Use :data:`default_io` directly instead of calling this function.

    Returns:
        The global default_io registry instance.
    """
    return default_io
