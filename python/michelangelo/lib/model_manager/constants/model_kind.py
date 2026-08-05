"""Model kind constants."""


class ModelKind:
    """Model kinds describing the prediction task a registered model performs.

    Attributes:
        CUSTOM: Model type not covered by the other kinds.
        REGRESSION: Regression model.
        BINARY_CLASSIFICATION: Binary classification model.
        MULTICLASS_CLASSIFICATION: Multiclass classification model.
    """

    CUSTOM = "custom"
    REGRESSION = "regression"
    BINARY_CLASSIFICATION = "binary-classification"
    MULTICLASS_CLASSIFICATION = "multiclass-classification"
