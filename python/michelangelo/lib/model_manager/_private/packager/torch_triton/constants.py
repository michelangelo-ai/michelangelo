"""File-name constants for the torch_triton packager."""

# Shared constants (used in both deployable and raw packages).
MODEL_PT_FILE_NAME = "model.pt"
MODEL_CLASS_FILE_NAME = "model_class.txt"

# Deployable package constants.
DEPLOYABLE_MODEL_ONNX_FILE_NAME = "model.onnx"
DEPLOYABLE_CONFIG_FILE_NAME = "config.pbtxt"
DEPLOYABLE_MODEL_PY_FILE_NAME = "model.py"
DEPLOYABLE_USER_MODEL_PY_FILE_NAME = "user_model.py"
DEPLOYABLE_MODEL_METADATA_FILE_NAME = "model_metadata.json"
DEPLOYABLE_SKELETON_FILE_NAME = "skeleton.yaml"

# Raw package constants.
RAW_TYPE_FILE_NAME = "type.yaml"
RAW_SCHEMA_FILE_NAME = "schema.yaml"
RAW_SAMPLE_DATA_FILE_NAME = "sample_data.json"
RAW_HYPERPARAMETERS_FILE_NAME = "hyperparameters.yaml"
RAW_REQUIREMENTS_FILE_NAME = "requirements.txt"
RAW_SUBMODEL_SCHEMAS_FILE = "submodel_schemas.yaml"
