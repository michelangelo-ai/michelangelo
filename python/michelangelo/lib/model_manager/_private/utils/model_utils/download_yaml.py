import os
import yaml
from uber.ai.michelangelo.sdk.model_manager.constants import StorageType
from uber.ai.michelangelo.sdk.model_manager.utils.terrablob_paths import get_raw_model_main_path


def convert_download_yamls_to_deployable(model_path: str, project_name: str, model_name: str, model_revision: str):
    """
    Convert all download.yaml files in the model path to deployable format

    Args:
        model_path: The local path of the deployable model package
        project_name: The project name
        model_name: The model name
        model_revision: The model revision
    """
    for dirpath, _, filenames in os.walk(model_path):
        for file in filenames:
            if file == "download.yaml":
                convert_to_deployable_download_yaml(os.path.join(dirpath, file), project_name, model_name, model_revision)


def convert_to_deployable_download_yaml(file_path: str, project_name: str, model_name: str, model_revision: str):
    """
    Convert the download.yaml to deployable format

    Args:
        file_path: The download.yaml file path
        project_name: The project name
        model_name: The model name
        model_revision: The model revision
    """
    with open(file_path, "r+") as f:
        content = yaml.safe_load(f)
        if not is_deployable_download_yaml_content(content):
            new_content = convert_to_deployable_download_yaml_content(content, project_name, model_name, model_revision)
            f.seek(0)
            yaml.dump(new_content, f)
            f.truncate()


def convert_to_deployable_download_yaml_content(content: dict, project_name: str, model_name: str, model_revision: str) -> dict:
    """
    Convert the download.yaml content to deployable format

    Example:

    assets:
    - a: root/a
    - b: root/b
    source_type: hdfs
    source_prefix: root/
    ->
    assets:
    - a: /prod/michelangelo/raw_models/projects/<project_name>/models/<model_name>/revisions/<model_revision>/main/model/a
    - b: /prod/michelangelo/raw_models/projects/<project_name>/models/<model_name>/revisions/<model_revision>/main/model/b
    source_type: terrablob
    source_prefix: /prod/michelangelo/raw_models/projects/<project_name>/models/<model_name>/revisions/<model_revision>/main/model/

    Args:
        content: The download.yaml content
        project_name: The project name
        model_name: The model name
        model_revision: The model revision

    Returns:
        The deployable download.yaml content
    """
    if is_deployable_download_yaml_content(content):
        return content

    res = dict(content)

    raw_model_main_path = get_raw_model_main_path(project_name, model_name, model_revision)
    raw_model_prefix = f"{raw_model_main_path}/model/"

    source_prefix = res.get("source_prefix")

    if "assets" in res:
        res_assets = []

        for asset in res["assets"]:
            res_asset = {}
            for key, value in asset.items():
                if not source_prefix:
                    res_asset[key] = f"{raw_model_prefix}{value}"
                else:
                    res_asset[key] = value.replace(source_prefix, raw_model_prefix)
            res_assets.append(res_asset)

        res["assets"] = res_assets

    res["source_type"] = StorageType.TERRABLOB

    if res.get("assets") and "source_prefix" in res:
        res["source_prefix"] = raw_model_prefix

    return res


def is_deployable_download_yaml_content(content: dict) -> bool:
    """
    Check if the download.yaml content is deployable

    Args:
        content: The download.yaml content

    Returns:
        True if the download.yaml content is deployable
    """
    return "source_type" not in content or content["source_type"] == StorageType.TERRABLOB
