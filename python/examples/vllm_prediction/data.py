import logging

import datasets
import ray
import transformers
from ray.data import Dataset

from michelangelo.uniflow.plugins.ray import RayTask
import michelangelo.uniflow.core as uniflow

log = logging.getLogger(__name__)


@uniflow.task(config=RayTask(
    head_cpu=1,
    head_memory="2Gi",
    worker_cpu=1,
    worker_memory="2Gi",
    worker_instances=1,
    #breakpoint=True,
    ))
def load_data(
    path: str,
    name: str,
    data_slice: str,
) -> tuple[Dataset, Dataset, Dataset]:
    hf_dataset = datasets.load_dataset(path=path, name=name)
    return ray.data.from_huggingface(hf_dataset[data_slice])
