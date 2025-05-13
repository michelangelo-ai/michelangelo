import os
import yaml
import shutil
from typing import Optional
from michelangelo._internal.gateways.terrablob_gateway import get_blob_info
from michelangelo._internal.errors.terrablob_error import TerrablobFileNotFoundError, TerrablobFailedPreconditionError
from michelangelo.lib.model_manager.constants import StorageType, PackageType
from michelangelo.lib.model_manager._private.utils.asset_utils import download_assets
from michelangelo.lib.model_manager._private.utils.terrablob_utils import get_terrablob_auth_mode


def download_assets_given_download_yaml(donwload_yaml_path: str, target_dir: str, timeout: Optional[str] = None, source_entity: Optional[str] = None):
    """
    Download the assets from the download yaml file to the target directory

    Args:
        donwload_yaml_path: The path to the download yaml file
        target_dir: The directory to store the assets
        timeout: The timeout for terrablob command. Defaults to None.
        source_entity: The source entity for terrablob command. Defaults to None.
    """
    with open(donwload_yaml_path) as f:
        yaml_content = yaml.safe_load(f)

    source_type = yaml_content.get("source_type", StorageType.TERRABLOB)
    for asset in yaml_content.get("assets", []):
        for des, src in asset.items():
            target = os.path.join(target_dir, des)
            parent = os.path.dirname(target)
            if parent:
                os.makedirs(parent, exist_ok=True)
            download_assets(src, target, source_type, timeout=timeout, source_entity=source_entity)


def validate_deployable_download_yaml(download_yaml_path: str, timeout: Optional[str] = None, source_entity: Optional[str] = None):
    """
    Validate the download yaml file

    Args:
        download_yaml_path: The path to the download yaml file
        timeout: The timeout for terrablob command. Defaults to None.
        source_entity: The source entity for terrablob command. Defaults to None.
    """
    with open(download_yaml_path) as f:
        yaml_content = yaml.safe_load(f)

    source_type = yaml_content.get("source_type")
    if source_type and source_type != StorageType.TERRABLOB:
        raise ValueError(f"Remote assets must be stored in Terrablob in deployable package, but got source_type: {source_type} in download.yaml")

    for asset in yaml_content.get("assets", []):
        for src in asset.values():
            try:
                get_blob_info(src, timeout=timeout, source_entity=source_entity, auth_mode=get_terrablob_auth_mode())
            except TerrablobFileNotFoundError as e:
                raise ValueError(f"Asset {src} does not exist in Terrablob") from e
            except TerrablobFailedPreconditionError as e:
                raise ValueError(f"Asset {src} is a directory, but expecting a file") from e


def validate_deployable_model_assets(model_path: str, timeout: Optional[str] = None, source_entity: Optional[str] = None):
    """
    Validate the remote downloadable assets in a deployable model package

    Args:
        model_path: The path to the model package
        timeout: The timeout for terrablob command. Defaults to None.
        source_entity: The source entity for terrablob command. Defaults to None.
    """
    for dirpath, _, filenames in os.walk(model_path):
        for filename in filenames:
            if filename == "download.yaml":
                download_yaml_path = os.path.join(dirpath, filename)
                try:
                    validate_deployable_download_yaml(download_yaml_path, timeout=timeout, source_entity=source_entity)
                except Exception as e:
                    raise RuntimeError(
                        "Error validating remote assets in the deployable model package. Make sure you have uploaded the raw model package. "
                        "Example: \n\n"
                        "   packager = XXPackager()\n"
                        "   raw_pkg = packager.create_raw_model_package(...)\n\n"
                        "Include the raw_pkg in the upload function: \n\n"
                        "   upload_model(deployable_pkg, raw_pkg, ...), or \n"
                        "   upload_raw_model(raw_pkg, ...)\n\n"
                        f"Error in download.yaml file: {download_yaml_path}. Error: {e}"
                    ) from e


def convert_assets_to_download_yaml(model_path: str, package_type: str, source_type: str, source_prefix: str):
    """
    Convert the model assets (binaries) in the model package to download.yaml

    Args:
        model_path: The path to the model package
        package_type: The package type of the model package
        source_type: The source type of the assets
        source_prefix: The source prefix of the assets
    """
    yaml_path = os.path.join(model_path, "0", "download.yaml")
    assets_path = os.path.join(model_path, "0", "model")

    if package_type not in PackageType.TRITON or os.path.exists(yaml_path) or not os.path.isdir(assets_path):
        return None

    def join_path(dirpath: str, filename: str) -> str:
        return os.path.relpath(os.path.join(dirpath, filename), assets_path).replace("\\", "/")

    assets = [
        {f"model/{join_path(dirpath, filename)}": f"{source_prefix}{join_path(dirpath, filename)}"}
        for dirpath, _, filenames in os.walk(assets_path)
        for filename in filenames
    ]

    yaml_content = {
        "assets": assets,
        "source_type": source_type,
        "source_prefix": source_prefix,
    }

    with open(yaml_path, "w") as f:
        yaml.dump(yaml_content, f)

    shutil.rmtree(assets_path)
