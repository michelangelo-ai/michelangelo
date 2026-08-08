"""Output schema reordering helpers for the torch tabular assembler."""

from __future__ import annotations

from michelangelo.lib.model_manager.schema import ModelSchema


def reorder_output_schema(
    schema: ModelSchema, field_order: list[str] | None
) -> ModelSchema:
    """Return a copy of ``schema`` with its output fields reordered.

    Fields named in ``field_order`` are placed first, in that order. Any
    output fields not covered by ``field_order`` are appended at the end,
    keeping their relative order.

    Args:
        schema: Schema whose ``output_schema`` should be reordered.
        field_order: Desired output field name order, or ``None`` to leave
            ``schema`` unchanged.

    Returns:
        ``schema`` unchanged when ``field_order`` is ``None``; otherwise a
        new ``ModelSchema`` with the same ``input_schema`` and a reordered
        ``output_schema``.
    """
    if field_order is None:
        return schema
    schema_by_name = {item.name: item for item in schema.output_schema}
    reordered = [schema_by_name[f] for f in field_order if f in schema_by_name]
    covered = set(field_order)
    reordered += [item for item in schema.output_schema if item.name not in covered]
    return ModelSchema(input_schema=list(schema.input_schema), output_schema=reordered)
