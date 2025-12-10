from uber.ai.michelangelo.experimental.model_manager_playground.python_triton_model.package.fn2 import (
    fn2,
)


def fn1():
    return f"package.fn1 and {fn2()}"
