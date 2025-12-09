import logging
import os
import uuid
from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import Any, Optional

from michelangelo.uniflow.core import IO

_logger = logging.getLogger(__name__)


@dataclass
class Variable(ABC):
    """Variable is an abstraction for an intermediate result of a workflow.
    Variable should describe high level concepts, such as dataset and model.

    It contains metadata as a dataclass, which is visible to the workflow engine for control flow,
    as well as subsquent tasks to pass meta information of the variable.

    It also contains generic methods for loading and saving a value, which is the actual data of the variable,
    such as a Spark DataFrame or a PyTorch model.
    Variable can invoke IO reflectively to load and save the value, which is used to avoid hard dependencies
    on Ray or Spark.
    """

    path: str = None
    metadata: Optional[Any] = None
    _io_metadata: Optional[Any] = None

    def __post_init__(self):
        self._value = None  # transient
        self._saved = False  # transient

    @classmethod
    def create(cls, value) -> "Variable":
        """A factory method to create a variable with the given value."""
        path = (
            f"{os.environ.get('UF_STORAGE_URL', 'memory://storage')}/{uuid.uuid4().hex}"
        )
        res = cls(path=path, metadata=None)
        res._value = value
        return res

    @abstractmethod
    def save(self):
        """Automatically find the IO class to save the value.
        It will be called by the workflow framework by the end of each task.
        This method should be implemented by subclasses.
        """

    @property
    def value(self) -> Any:
        if self._value is None:
            self._load()
        return self._value

    @abstractmethod
    def _load(self):
        """Automatically find the IO class to load the value.
        Should not be called directly. Use `value` property instead.
        This method should be implemented by subclasses.
        """

    def _load_value_using_io(self, io_class: type):
        """A helper method to load the value located at the variable's path using the given IO.
        IO can be given as either as a concrete instance, or as a string representing a dot-path to the IO's class.
        """
        _logger.info(f"loading value for {self.path}")

        if self._value is not None:
            _logger.info(f"value already loaded for {self.path}. Skipping loading.")
            return

        io = _create_io(io_class)

        self._value = io.read(self.path, self._io_metadata)
        self._saved = True  # the value is already saved on disk

    def _save_value_using_io(self, io_class: type):
        """A helper method to save the value using the given IO class.
        IO can be given as either as a concrete instance, or as a string representing a dot-path to the IO's class.
        """
        _logger.info(f"saving value for {self.path}")

        if self._saved:
            _logger.info(f"value already saved for {self.path}. Skipping saving.")
            return

        io = _create_io(io_class)
        self._io_metadata = io.write(self.path, self._value)
        self._saved = True


def _create_io(io_class: type) -> IO:
    """A helper method to create IO from class."""
    io = io_class()
    assert isinstance(io, IO)
    return io
