import numpy as np
from .dep import B


class A:
    def method(self):
        return np.array([1, 2, 3])


def func():
    return B(1).method()
