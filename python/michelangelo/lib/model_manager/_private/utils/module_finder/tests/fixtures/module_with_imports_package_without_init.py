# flake8: noqa:TID252
from .simple_module import module_attr
from .package import fn1 as pfn1
from .folder import (
    fn1 as afn1,
    fn2,
)


def fn():
    afn1()
    fn2.fn2()
    pfn1()
    module_attr()
