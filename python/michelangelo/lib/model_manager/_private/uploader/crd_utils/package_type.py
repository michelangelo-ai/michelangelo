from michelangelo.lib.model_manager.constants import PackageType
from michelangelo.gen.api.v2.model_pb2 import (
    DeployableModelPackageType,
    DEPLOYABLE_MODEL_PACKAGE_TYPE_SPARK_PIPELINE,
    DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON,
    DEPLOYABLE_MODEL_PACKAGE_TYPE_RAW,
    DEPLOYABLE_MODEL_PACKAGE_TYPE_MOBILE,
)


def convert_package_type(package_type: str) -> DeployableModelPackageType:
    """
    Convert the package type from the model schema to the proto package type in Unified API.

    Args:
        package_type: Package type from the model schema.

    Returns:
        Proto package type in Unified API.
    """
    mapping = {
        PackageType.SPARK: DEPLOYABLE_MODEL_PACKAGE_TYPE_SPARK_PIPELINE,
        PackageType.TRITON: DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON,
        PackageType.RAW: DEPLOYABLE_MODEL_PACKAGE_TYPE_RAW,
        PackageType.MOBILE: DEPLOYABLE_MODEL_PACKAGE_TYPE_MOBILE,
    }
    return mapping.get(package_type, DeployableModelPackageType.DEPLOYABLE_MODEL_PACKAGE_TYPE_INVALID)
