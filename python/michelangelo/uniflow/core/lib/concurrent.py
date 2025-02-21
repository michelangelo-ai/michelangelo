import atexit
from concurrent.futures import ProcessPoolExecutor

from michelangelo.uniflow.core import star_plugin
from abc import ABC, abstractmethod

# TODO: andrii: revise concurrent execution for local runs.
# Currently, ProcessPoolExecutor fails to serialize TaskFunction resulting in the following error:
#   _pickle.PicklingError: Can't pickle <function task_1 at 0x7bbf8db623a0>: it's not the same object as __main__.task_1

_USE_PROCESS_POOL = False

_pool = None


class Future(ABC):
    @abstractmethod
    def result(self):
        raise NotImplementedError


class _Future(Future):
    def __init__(self, source):
        self._source = source

    def result(self):
        return self._source.result()


class _ResolvedFuture(Future):
    def __init__(self, result):
        self._result = result

    def result(self):
        return self._result


@star_plugin("concurrent.run")
def run(fn, *args, **kwargs) -> Future:
    if _USE_PROCESS_POOL:
        return _process_pool_run(fn, *args, **kwargs)

    # Fake concurrency for local runs - run in the current process.
    result = fn(*args, **kwargs)
    return _ResolvedFuture(result)


def _process_pool_run(fn, *args, **kwargs) -> Future:
    global _pool
    if not _pool:
        _pool = ProcessPoolExecutor()
        atexit.register(_pool.shutdown)

    f = _pool.submit(fn, *args, **kwargs)
    return _Future(f)
