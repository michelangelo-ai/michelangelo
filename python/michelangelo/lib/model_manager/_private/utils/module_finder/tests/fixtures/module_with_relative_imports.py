# flake8: noqa:TID252
from .simple_module import module_attr
from .folder.fn1 import fn1
from uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.folder.fn2 import fn2
from .package import fn1 as pfn1
import uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.folder.fn3 as fn3


def fn():
    fn1()
    fn2()
    fn3.fn3()
    pfn1()
    module_attr()
