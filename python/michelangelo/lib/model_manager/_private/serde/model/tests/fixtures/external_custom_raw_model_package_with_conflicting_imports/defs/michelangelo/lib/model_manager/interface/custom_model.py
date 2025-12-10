from __future__ import annotations
from abc import ABC, abstractmethod
from numpy import ndarray  # noqa: TC002


class Model(ABC):
    """
    The base class for custom models.
    """

    @abstractmethod
    def save(self, path: str):
        """
        Save the model to the given path.
        Strongly recommend to not use pickle or torch.save directly.

        Args:
            path: The local path to save the model.

        Returns:
            None
        """

    @classmethod
    @abstractmethod
    def load(cls, path: str) -> Model:
        """
        Load the model from the given path.

        Args:
            path: The local path to load the model from.

        Returns:
            The model instance.
        """
        raise NotImplementedError("load method is not implemented")

    @abstractmethod
    def predict(
        self,
        inputs: dict[str, ndarray],
    ) -> dict[str, ndarray]:
        """
        Predict on the given data.

        Args:
            inputs: The input data for prediction.

        Returns:
            The prediction result.
        """
