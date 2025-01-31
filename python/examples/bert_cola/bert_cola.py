import os

import ray
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask, UF_PLUGIN_RAY_USE_FSSPEC


@uniflow.task(config=RayTask(head_cpu=4))
def load_data(shape: tuple[int, int]):
    n = os.environ["DATA_SIZE"]
    data = ray.data.range_tensor(int(n), shape=shape)
    print(data.schema())
    return data


@uniflow.workflow()
def train_workflow(data_shape: tuple[int, int]):
    data = load_data(shape=data_shape)
    print("data:", data)
    print("ok.")

# For Local Run: python3 examples/bert_cola/bert_cola.py
# For Remote Run: python3 examples/bert_cola/bert_cola.py remote-run --storage-url <STORAGE_URL> --image <IMAGE>
if __name__ == "__main__":

    ctx = uniflow.create_context()

    # Set the environment variable DATA_SIZE to let the load_data task know how much data to generate.
    ctx.environ["DATA_SIZE"] = "10"

    # Disable use of fsspec in Ray Plugin. See UF_PLUGIN_RAY_USE_FSSPEC docstring for more information.
    ctx.environ[UF_PLUGIN_RAY_USE_FSSPEC] = "0"
    ctx.run(train_workflow, data_shape=(64, 64))
