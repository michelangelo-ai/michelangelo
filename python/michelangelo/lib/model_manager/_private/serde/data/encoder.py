"""Custom JSON encoder for model input/output data."""

from json import JSONEncoder

from numpy import ndarray


class DataEncoder(JSONEncoder):
    """Custom JSON encoder for model input/output data."""

    def default(self, obj: any) -> any:
        """Encode the object to a JSON-serializable dictionary."""
        if isinstance(obj, ndarray):
            return obj.tolist()

        if isinstance(obj, bytes):
            return obj.decode("utf-8")

        return JSONEncoder.default(self, obj)
