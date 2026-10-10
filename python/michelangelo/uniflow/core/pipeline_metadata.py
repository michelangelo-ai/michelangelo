"""Pipeline metadata model, validation and pipeline-type normalization.

The metadata mirrors the user-authored fields of a Michelangelo ``Pipeline``
CR (``metadata.name``, ``metadata.labels``, ``spec.owner.name``,
``spec.manifest.triggerMap``, ...) so a workflow can declare them inline, as
``@uniflow.workflow(name=..., ...)`` arguments, instead of in a separate
pipeline YAML such as ``pipeline.yaml``.
"""

import dataclasses
import difflib
import importlib
import inspect
import os
import re
from collections.abc import Mapping, Sequence
from typing import Any, Callable, Optional, Protocol

PIPELINE_TYPE_PREFIX = "PIPELINE_TYPE_"
DEFAULT_PIPELINE_TYPE = "PIPELINE_TYPE_TRAIN"
UNIFLOW_IMAGE_ANNOTATION = "michelangelo/uniflow-image"
_INVALID_PIPELINE_TYPE = "PIPELINE_TYPE_INVALID"
_DEFAULT_PB2_MODULE = "michelangelo.gen.api.v2.pipeline_pb2"

# Generated module of each proto message that trigger and notification values
# are checked against.
_PROTO_MESSAGE_MODULES = {
    "Trigger": "michelangelo.gen.api.v2.trigger_run_pb2",
    "Notification": "michelangelo.gen.api.v2.notification_pb2",
}

STRING_FIELDS = (
    "name",
    "namespace",
    "owner",
    "description",
    "type",
    "image",
    "git_ref",
    "branch",
)
MAP_FIELDS = ("labels", "annotations", "triggers")
LIST_FIELDS = ("notifications",)

# Attribute set on the workflow function (and its unwrapped original).
METADATA_ATTR = "_uf_pipeline"

_DNS1123_LABEL = re.compile(r"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$")
_DNS1123_SUBDOMAIN = re.compile(
    r"^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"
)
_DNS1123_LABEL_MAX = 63
_DNS1123_SUBDOMAIN_MAX = 253
# Kubernetes label/annotation key name part, and label values.
_QUALIFIED_NAME = re.compile(r"^([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]$")
_QUALIFIED_NAME_MAX = 63


class PipelineMetadataError(ValueError):
    """Raised when inline pipeline metadata is missing or invalid."""


class PipelineTypeCatalog(Protocol):
    """Source of valid ``PipelineType`` enum names."""

    def names(self) -> frozenset[str]:
        """Return every declared ``PIPELINE_TYPE_*`` enum member name."""


class ProtoPipelineTypeCatalog:
    """Reads ``PipelineType`` names from the generated ``pipeline_pb2`` module.

    The import is lazy so that importing this module (and therefore
    ``michelangelo.uniflow.core``, including in task containers) doesn't pay
    for, or fail on, the generated proto until a ``type`` needs validating.
    """

    def __init__(self, module_path: str = _DEFAULT_PB2_MODULE):
        """Initialize the catalog.

        Args:
            module_path: Dotted path of the generated ``pipeline_pb2`` module.
        """
        self._module_path = module_path

    def names(self) -> frozenset[str]:
        """Return every declared ``PIPELINE_TYPE_*`` enum member name."""
        pb2 = importlib.import_module(self._module_path)
        return frozenset(v.name for v in pb2.PipelineType.DESCRIPTOR.values)


class MessageSchema(Protocol):
    """Checks CR values against the proto message they are parsed into."""

    def check(self, message: str, value: Mapping[str, Any]) -> Optional[str]:
        """Return why ``value`` is not a valid ``message``, or ``None``."""


class ProtoMessageSchema:
    """Checks values against the generated protos, imported lazily.

    Values use the CR's JSON (camelCase) field names, as in a pipeline YAML.
    The original snake_case names are rejected: protobuf would accept them,
    but CR tooling such as mactl's trigger conversion only reads camelCase.
    """

    def check(self, message: str, value: Mapping[str, Any]) -> Optional[str]:
        """Return why ``value`` is not a valid ``message``, or ``None``."""
        from google.protobuf import json_format

        pb2 = importlib.import_module(_PROTO_MESSAGE_MODULES[message])
        cls = getattr(pb2, message)
        problem = _snake_case_key(value, cls.DESCRIPTOR)
        if problem:
            return problem
        try:
            json_format.ParseDict(value, cls())
        except json_format.ParseError as e:
            return str(e)
        return None


def normalize_pipeline_type(
    value: str, catalog: Optional[PipelineTypeCatalog] = None
) -> str:
    """Return the full ``PIPELINE_TYPE_*`` name for ``value``.

    Accepts the full enum name or its short suffix, case-insensitively
    (``train``, ``TRAIN`` and ``PIPELINE_TYPE_TRAIN`` are equivalent).

    Args:
        value: The pipeline type as written by the author.
        catalog: Source of valid names. Defaults to the generated proto.

    Returns:
        The full enum member name, e.g. ``PIPELINE_TYPE_TRAIN``.

    Raises:
        PipelineMetadataError: If ``value`` is not a declared, valid member.
    """
    candidate = value.strip().upper()
    if not candidate.startswith(PIPELINE_TYPE_PREFIX):
        candidate = PIPELINE_TYPE_PREFIX + candidate
    names = (catalog or ProtoPipelineTypeCatalog()).names()
    if candidate in names and candidate != _INVALID_PIPELINE_TYPE:
        return candidate

    valid = sorted(
        n[len(PIPELINE_TYPE_PREFIX) :] for n in names if n != _INVALID_PIPELINE_TYPE
    )
    message = f'type "{value}" is not a valid pipeline type'
    close = difflib.get_close_matches(
        candidate[len(PIPELINE_TYPE_PREFIX) :], valid, n=1
    )
    if close:
        message += f'; did you mean "{close[0]}"?'
    raise PipelineMetadataError(f"{message} Expected one of: {', '.join(valid)}")


@dataclasses.dataclass(frozen=True)
class PipelineMetadata:
    """User-authored Pipeline CR fields declared alongside a workflow.

    Every field is optional at declaration time. ``name`` and ``namespace``
    are required once the CR is resolved, but a pipeline YAML may still supply
    them while a pipeline migrates to inline metadata.

    The remaining CR fields are filled in by registration tooling:
    ``spec.manifest.filePath``/``type``/``uniflowFunction`` (from the workflow
    module) and ``spec.manifest.uniflowTar``/``content`` (from the upload).

    Attributes:
        name: ``metadata.name`` (DNS-1123 subdomain).
        namespace: ``metadata.namespace`` (DNS-1123 label).
        owner: ``spec.owner.name``.
        description: ``spec.description``.
        type: ``spec.type``; short (``TRAIN``) or full (``PIPELINE_TYPE_TRAIN``).
        image: ``metadata.annotations["michelangelo/uniflow-image"]``.
        git_ref: ``spec.commit.gitRef``; defaults to the current git commit.
        branch: ``spec.commit.branch``; defaults to the current git branch.
        labels: ``metadata.labels``.
        annotations: ``metadata.annotations``, besides ``image``.
        triggers: ``spec.manifest.triggerMap``: trigger name to a ``Trigger``
            in CR form, e.g. ``{"cronSchedule": {"cron": "0 8 * * *"},
            "maxConcurrency": 1}``.
        notifications: ``spec.notifications``: ``Notification`` values in CR
            form, e.g. ``{"notificationType": "NOTIFICATION_TYPE_EMAIL", ...}``.
    """

    name: Optional[str] = None
    namespace: Optional[str] = None
    owner: Optional[str] = None
    description: Optional[str] = None
    type: Optional[str] = None
    image: Optional[str] = None
    git_ref: Optional[str] = None
    branch: Optional[str] = None
    labels: Optional[Mapping[str, str]] = None
    annotations: Optional[Mapping[str, str]] = None
    triggers: Optional[Mapping[str, Mapping[str, Any]]] = None
    notifications: Optional[Sequence[Mapping[str, Any]]] = None

    def validate(
        self,
        catalog: Optional[PipelineTypeCatalog] = None,
        schema: Optional[MessageSchema] = None,
    ) -> list[str]:
        """Return human-readable problems with the declared values.

        Args:
            catalog: Source of valid pipeline types. Defaults to the proto.
            schema: Checks triggers and notifications. Defaults to the protos.

        Returns:
            One message per problem; empty when the metadata is valid.
        """
        errors = self._shape_errors()
        if errors:
            return errors

        if self.name is not None and not _is_dns1123(
            self.name, _DNS1123_SUBDOMAIN, _DNS1123_SUBDOMAIN_MAX
        ):
            errors.append(
                f'name "{self.name}" must be a DNS-1123 subdomain (lowercase '
                "alphanumerics, '-' or '.', starting and ending alphanumeric)"
            )
        if self.namespace is not None and not _is_dns1123(
            self.namespace, _DNS1123_LABEL, _DNS1123_LABEL_MAX
        ):
            errors.append(
                f'namespace "{self.namespace}" must be a DNS-1123 label '
                "(lowercase alphanumerics or '-', at most 63 characters)"
            )
        for field in ("owner", "git_ref", "branch"):
            value = getattr(self, field)
            if value is not None and re.search(r"\s", value):
                errors.append(f'{field} "{value}" must not contain whitespace')
        if self.type is not None:
            try:
                normalize_pipeline_type(self.type, catalog)
            except PipelineMetadataError as e:
                errors.append(str(e))
        errors += _key_errors("labels", self.labels, check_values=True)
        errors += _key_errors("annotations", self.annotations, check_values=False)
        image = (self.annotations or {}).get(UNIFLOW_IMAGE_ANNOTATION)
        if self.image is not None and image is not None and image != self.image:
            errors.append(
                f'image "{self.image}" conflicts with '
                f'annotations["{UNIFLOW_IMAGE_ANNOTATION}"] "{image}"; set only one'
            )
        schema = schema or ProtoMessageSchema()
        for trigger, value in (self.triggers or {}).items():
            problem = schema.check("Trigger", value)
            if problem:
                errors.append(f'triggers["{trigger}"]: {problem}')
        for i, value in enumerate(self.notifications or ()):
            problem = schema.check("Notification", value)
            if problem:
                errors.append(f"notifications[{i}]: {problem}")
        return errors

    def normalized(
        self, catalog: Optional[PipelineTypeCatalog] = None
    ) -> "PipelineMetadata":
        """Return a copy with ``type`` expanded and containers copied.

        ``type`` becomes its full enum name, and mappings and sequences become
        plain dicts and lists, so later changes by the caller don't leak in.

        Args:
            catalog: Source of valid pipeline types. Defaults to the proto.

        Returns:
            The normalized metadata.

        Raises:
            PipelineMetadataError: If ``type`` is invalid.
        """
        updates: dict[str, Any] = {}
        if self.type is not None:
            updates["type"] = normalize_pipeline_type(self.type, catalog)
        for field in MAP_FIELDS + LIST_FIELDS:
            value = getattr(self, field)
            if value is not None:
                updates[field] = _plain(value)
        return dataclasses.replace(self, **updates) if updates else self

    def as_dict(self) -> dict[str, Any]:
        """Return the fields as a plain dict (``None`` for unset fields)."""
        return dataclasses.asdict(self)

    def _shape_errors(self) -> list[str]:
        """Return problems with the Python types of the declared values."""
        errors = []
        for field in STRING_FIELDS:
            value = getattr(self, field)
            if value is None:
                continue
            if not isinstance(value, str):
                errors.append(f"{field} must be a string, got {_type_name(value)}")
            elif not value.strip():
                errors.append(f"{field} must not be empty")
        for field in ("labels", "annotations"):
            value = getattr(self, field)
            if value is None:
                continue
            if not isinstance(value, Mapping):
                errors.append(f"{field} must be a mapping, got {_type_name(value)}")
                continue
            for key, item in value.items():
                if not isinstance(key, str) or not isinstance(item, str):
                    errors.append(f"{field} keys and values must be strings")
                    break
        if self.triggers is not None:
            if not isinstance(self.triggers, Mapping):
                errors.append(
                    f"triggers must be a mapping, got {_type_name(self.triggers)}"
                )
            else:
                for trigger, value in self.triggers.items():
                    if not isinstance(trigger, str) or not trigger.strip():
                        errors.append("trigger names must be non-empty strings")
                    elif not isinstance(value, Mapping):
                        errors.append(
                            f'triggers["{trigger}"] must be a mapping, '
                            f"got {_type_name(value)}"
                        )
        if self.notifications is not None:
            if isinstance(self.notifications, (str, bytes, Mapping)) or not (
                isinstance(self.notifications, Sequence)
            ):
                errors.append(
                    "notifications must be a list, "
                    f"got {_type_name(self.notifications)}"
                )
            else:
                for i, value in enumerate(self.notifications):
                    if not isinstance(value, Mapping):
                        errors.append(
                            f"notifications[{i}] must be a mapping, "
                            f"got {_type_name(value)}"
                        )
        return errors


def describe_function(fn: Callable[..., Any]) -> str:
    """Return ``qualname (file:line)`` for error messages about ``fn``."""
    target = inspect.unwrap(fn)
    code = getattr(target, "__code__", None)
    name = getattr(target, "__qualname__", repr(target))
    if code is None:
        return name
    filename = code.co_filename
    try:
        relative = os.path.relpath(filename)
        if not relative.startswith(".."):
            filename = relative
    except ValueError:
        pass
    return f"{name} ({filename}:{code.co_firstlineno})"


def get_pipeline_metadata(fn: Callable[..., Any]) -> Optional[PipelineMetadata]:
    """Return the inline pipeline metadata attached to ``fn``, if any.

    Args:
        fn: A workflow function, wrapped or unwrapped.

    Returns:
        The attached metadata, or ``None`` when ``fn`` declares none.
    """
    meta = getattr(fn, METADATA_ATTR, None)
    if meta is None:
        meta = getattr(inspect.unwrap(fn), METADATA_ATTR, None)
    return meta


def _is_dns1123(value: str, pattern: re.Pattern, max_len: int) -> bool:
    return len(value) <= max_len and bool(pattern.match(value))


def _is_qualified_name(key: str) -> bool:
    """Whether ``key`` is a valid Kubernetes label/annotation key."""
    prefix, _, name = key.rpartition("/")
    if prefix and not _is_dns1123(prefix, _DNS1123_SUBDOMAIN, _DNS1123_SUBDOMAIN_MAX):
        return False
    return _is_dns1123(name, _QUALIFIED_NAME, _QUALIFIED_NAME_MAX)


def _key_errors(
    field: str, values: Optional[Mapping[str, str]], check_values: bool
) -> list[str]:
    errors = []
    for key, value in (values or {}).items():
        if not _is_qualified_name(key):
            errors.append(
                f'{field} key "{key}" must be a Kubernetes qualified name '
                "([prefix/]name; name at most 63 alphanumerics, '-', '_' or '.')"
            )
        elif (
            check_values
            and value
            and not _is_dns1123(value, _QUALIFIED_NAME, _QUALIFIED_NAME_MAX)
        ):
            errors.append(
                f'{field}["{key}"] value "{value}" must be at most 63 '
                "alphanumerics, '-', '_' or '.', starting and ending alphanumeric"
            )
    return errors


def _snake_case_key(value: Any, descriptor: Any, path: str = "") -> Optional[str]:
    """Return a message for the first snake_case field name in ``value``."""
    if not isinstance(value, Mapping):
        return None
    by_json_name = {f.json_name: f for f in descriptor.fields}
    for key, item in value.items():
        where = f"{path}.{key}" if path else str(key)
        field = by_json_name.get(key)
        if field is None:
            snake = descriptor.fields_by_name.get(key)
            if snake is not None:
                return f'use "{snake.json_name}" instead of "{key}" at {where}'
        # Unknown names have no message to walk; ParseDict reports them.
        message = _user_message_type(field) if field is not None else None
        if message is None:
            continue
        if message.GetOptions().map_entry:
            value_type = _user_message_type(message.fields_by_name["value"])
            items = item.items() if value_type and isinstance(item, Mapping) else ()
            children = [(f"{where}.{k}", v) for k, v in items]
            message = value_type
        elif _is_repeated(field):
            items = item if isinstance(item, list) else []
            children = [(f"{where}[{i}]", v) for i, v in enumerate(items)]
        else:
            children = [(where, item)]
        for child_path, child in children:
            problem = _snake_case_key(child, message, child_path)
            if problem:
                return problem
    return None


def _user_message_type(field: Any) -> Any:
    """Return the field's message type, unless it's absent or well-known."""
    message = field.message_type
    if message is None or message.full_name.startswith("google.protobuf."):
        return None
    return message


def _is_repeated(field: Any) -> bool:
    # protobuf 7 removed FieldDescriptor.label in favour of is_repeated.
    is_repeated = getattr(field, "is_repeated", None)
    if is_repeated is not None:
        return bool(is_repeated)
    return field.label == field.LABEL_REPEATED


def _plain(value: Any) -> Any:
    """Return ``value`` with mappings and sequences copied as dicts and lists."""
    if isinstance(value, Mapping):
        return {k: _plain(v) for k, v in value.items()}
    if isinstance(value, (list, tuple)):
        return [_plain(v) for v in value]
    return value


def _type_name(value: Any) -> str:
    return type(value).__name__
