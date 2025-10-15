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

    def done(self) -> bool:
        raise NotImplementedError


class _Future(Future):
    def __init__(self, source):
        self._source = source

    def result(self):
        return self._source.result()

    def done(self) -> bool:
        return self._source.done()


class _ResolvedFuture(Future):
    def __init__(self, result):
        self._result = result

    def result(self):
        return self._result

    def done(self) -> bool:
        return True  # future is always resolved


class Callable(ABC):
    @abstractmethod
    def __call__(self):
        raise NotImplementedError


class _Callable(Callable):
    def __init__(self, fn, args):
        self.fn = fn
        self.args = args

    def __call__(self):
        return self.fn(*self.args)


class BatchFuture(ABC):
    @abstractmethod
    def is_ready(self) -> bool:
        raise NotImplementedError

    @abstractmethod
    def get(self) -> list[object]:
        raise NotImplementedError

    @abstractmethod
    def get_futures(self) -> list[Future]:
        raise NotImplementedError


class _BatchFuture(BatchFuture):
    def __init__(self, futures: list[Future]):
        self._futures = futures

    def is_ready(self) -> bool:
        return all(f.done() for f in self._futures)

    def get(self) -> list[object]:
        return [f.result() for f in self._futures]

    def get_futures(self) -> list[Future]:
        return [_Future(f) for f in self._futures]


@star_plugin("concurrent.run")
def run(fn, *args, **kwargs) -> Future:
    """
    Execute a function concurrently and return a Future.

    Args:
        fn: The function to execute
        *args: Positional arguments to pass to the function
        **kwargs: Keyword arguments to pass to the function

    Returns:
        Future object that will contain the result
    """
    if _USE_PROCESS_POOL:
        return _process_pool_run(fn, *args, **kwargs)

    # Fake concurrency for local runs - run in the current process.
    result = fn(*args, **kwargs)
    return _ResolvedFuture(result)


@star_plugin("concurrent.new_callable")
def new_callable(fn, *args):
    """
    Creates a callable object that wraps a function and its arguments.

    Args:
        fn: The function to wrap
        *args: Arguments to pass to the function when called

    Returns:
        A callable object that can be executed later
    """
    return _Callable(fn, args)


@star_plugin("concurrent.batch_run")
def batch_run(callables, max_concurrency=None) -> BatchFuture:
    """
    Execute multiple callables in parallel with optional concurrency limit.

    Args:
        callables: List of callable objects created by new_callable
        max_concurrency: Maximum number of concurrent executions (None = unlimited)

    Returns:
        BatchFuture object containing the results
    """
    if _USE_PROCESS_POOL:
        return _process_pool_batch_run(callables, max_concurrency)

    # Fake concurrency for local runs: execute sequentially
    results = []
    for callable_obj in callables:
        result = callable_obj()
        results.append(_ResolvedFuture(result))

    return _BatchFuture(results)


def _process_pool_batch_run(callables, max_concurrency=None) -> BatchFuture:
    """
    Execute callables using ProcessPoolExecutor.

    Args:
        callables: List of callable objects
        max_concurrency: Maximum number of workers

    Returns:
        BatchFuture with the execution futures
    """
    global _pool
    if not _pool:
        max_workers = max_concurrency if max_concurrency else None
        _pool = ProcessPoolExecutor(max_workers=max_workers)
        atexit.register(_pool.shutdown)

    futures = [_pool.submit(callable_obj) for callable_obj in callables]

    return _BatchFuture(futures)


def _process_pool_run(fn, *args, **kwargs) -> Future:
    global _pool
    if not _pool:
        _pool = ProcessPoolExecutor()
        atexit.register(_pool.shutdown)

    f = _pool.submit(fn, *args, **kwargs)
    return _Future(f)
