from json import JSONEncoder
from numpy import ndarray


class DataEncoder(JSONEncoder):
    """
    Custom JSON encoder for model input/output data
    """

    def default(self, obj: any) -> any:
        if isinstance(obj, ndarray):
            return obj.tolist()

        if isinstance(obj, bytes):
            return obj.decode("utf-8")

        return JSONEncoder.default(self, obj)
