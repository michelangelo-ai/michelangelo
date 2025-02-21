import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import UF_PLUGIN_RAY_USE_FSSPEC

import torch
import transformers
from transformers import AutoModelForCausalLM, AutoTokenizer, Trainer, TrainingArguments
from datasets import load_dataset


@uniflow.workflow()
def train_workflow():

    # Define model and tokenizer
    MODEL_NAME = "nomic-ai/gpt4all"  # Replace with a specific model from nomic-ai if needed
    tokenizer = AutoTokenizer.from_pretrained(MODEL_NAME)
    model = AutoModelForCausalLM.from_pretrained(MODEL_NAME)

    # Load dataset (example: wikitext)
    dataset = load_dataset("wikitext", "wikitext-2-raw-v1")

    # Tokenization function
    def tokenize_function(examples):
        return tokenizer(examples["text"], padding="max_length", truncation=True, max_length=512)

    # Tokenize dataset
    tokenized_datasets = dataset.map(tokenize_function, batched=True)
    tokenized_datasets = tokenized_datasets.remove_columns(["text"]).rename_column("input_ids", "labels")

    # Define training arguments
    training_args = TrainingArguments(
        output_dir="./results",
        evaluation_strategy="epoch",
        save_strategy="epoch",
        logging_dir="./logs",
        logging_steps=500,
        per_device_train_batch_size=4,
        per_device_eval_batch_size=4,
        num_train_epochs=3,
        weight_decay=0.01,
        save_total_limit=2,
        fp16=torch.cuda.is_available(),
        push_to_hub=False,  # Set to True if you want to upload the trained model
    )

    # Define Trainer
    trainer = Trainer(
        model=model,
        args=training_args,
        train_dataset=tokenized_datasets["train"],
        eval_dataset=tokenized_datasets["validation"],
        tokenizer=tokenizer,
    )

    # Train the model
    trainer.train()

    # Save final model
    model.save_pretrained("./trained_model")
    tokenizer.save_pretrained("./trained_model")
    print("ok.")


# For Local Run: python3 examples/bert_cola/bert_cola.py
# For Remote Run: python3 examples/bert_cola/bert_cola.py remote-run --storage-url <STORAGE_URL> --image <IMAGE>
if __name__ == "__main__":

    ctx = uniflow.create_context()

    # Set the environment variable DATA_SIZE to let the load_data task know how much data to generate.
    ctx.environ["DATA_SIZE"] = "10"

    # Disable use of fsspec in Ray Plugin. See UF_PLUGIN_RAY_USE_FSSPEC docstring for more information.
    ctx.environ[UF_PLUGIN_RAY_USE_FSSPEC] = "0"
    ctx.environ['PYTORCH_MPS_HIGH_WATERMARK_RATIO'] ='0'
    ctx.run(train_workflow)
