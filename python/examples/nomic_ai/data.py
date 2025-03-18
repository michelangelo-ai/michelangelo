import logging

from datasets import load_dataset
from ray.data import Dataset
from transformers import AutoTokenizer

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask

tokenizer_path = "bert-base-cased"

log = logging.getLogger(__name__)


@uniflow.task(config=RayTask(
    head_cpu=1,
    head_memory="2Gi",
    worker_cpu=1,
    worker_memory="2Gi",
    worker_instances=1,
    #breakpoint=True
    ))
def load_data(model_name="nomic-ai/nomic-bert-2048", dataset_name: str = "wikitext", tokenizer=None, max_length: int = 512,  dataset_size=200) -> tuple[Dataset, Dataset, Dataset]:
    tokenizer = AutoTokenizer.from_pretrained(model_name)

    dataset = load_dataset(dataset_name, "wikitext-2-raw-v1")

    def tokenize_function(examples):
        return tokenizer(examples["text"], padding="max_length", truncation=True, max_length=max_length)

    dataset = dataset.map(tokenize_function, batched=True)

    for split in ["train", "validation", "test"]:
        if split in dataset:
            dataset[split] = dataset[split].select(range(min(dataset_size, len(dataset[split]))))


    dataset.set_format(type="torch", columns=["input_ids", "attention_mask"])

    return (
        dataset["train"],
        dataset["validation"],
        dataset["test"],
    )
