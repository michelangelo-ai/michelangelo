import torch
import fsspec
import tempfile
from transformers import AutoTokenizer

# Define the S3 URI for the traced model
traced_model_s3_uri = "s3://deploy-models/bert_cola/bert_cola/1/model.pt"

# Load tokenizer
tokenizer = AutoTokenizer.from_pretrained("bert-base-cased")

# Download the traced model from S3 into a temporary file
with tempfile.NamedTemporaryFile(suffix=".pt") as tmp_file:
    with fsspec.open(traced_model_s3_uri, mode="rb") as s3_file:
        tmp_file.write(s3_file.read())
        tmp_file.flush()

    # Load the traced model
    model = torch.jit.load(tmp_file.name, map_location="cpu")
    model.eval()

    # Prepare input
    input_text = "This is an example sentence for local prediction."
    inputs = tokenizer(input_text, return_tensors="pt")
    input_ids = inputs["input_ids"]
    attention_mask = inputs["attention_mask"]

    # Perform inference
    with torch.no_grad():
        output = model(input_ids, attention_mask)

    # Print the output
    print("Model output:", output)
