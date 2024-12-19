import unittest
from michelangelo.task import task


@task(config="test")
def task_1(x):
    return x * x


class Test(unittest.TestCase):
    def test_1(self):
        result = task_1(2)
        self.assertEqual(4, result)
