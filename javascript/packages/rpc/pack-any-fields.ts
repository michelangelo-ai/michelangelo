import { create } from '@bufbuild/protobuf';
import {
  anyPack,
  BoolValueSchema,
  DoubleValueSchema,
  Int64ValueSchema,
  StringValueSchema,
} from '@bufbuild/protobuf/wkt';

import type { DescField, DescMessage } from '@bufbuild/protobuf';

const ANY_TYPE_NAME = 'google.protobuf.Any';

/**
 * Walks a request object against its proto descriptor, packing JS primitives into well-known
 * wrapper types wherever the schema has a `google.protobuf.Any` field.
 *
 * @example
 * packAnyFields(CriterionSchema, { fieldName: "x", matchValue: "my-pipeline" })
 * // matchValue -> anyPack(StringValueSchema, create(StringValueSchema, { value: "my-pipeline" }))
 */
export function packAnyFields(desc: DescMessage, value: unknown): unknown {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return value;

  const result: Record<string, unknown> = {};
  for (const [key, val] of Object.entries(value)) {
    const field: DescField | undefined = desc.field[key];
    result[key] = field ? packField(field, val) : val;
  }
  return result;
}

function packField(field: DescField, value: unknown): unknown {
  if (value === null || value === undefined) return value;

  switch (field.fieldKind) {
    case 'message':
      return packMessageValue(field.message, value);
    case 'list':
      if (field.listKind === 'message') {
        // cast: repeated fields are always arrays at runtime
        return (value as unknown[]).map((item) => packMessageValue(field.message, item));
      }
      return value;
    case 'map': {
      const mapValueMessage = field.message;
      if (!mapValueMessage) return value;
      // cast: map fields are always plain objects at runtime, keyed by the (stringified)
      // map key regardless of its declared scalar type
      const mapValue = value as Record<string, unknown>;
      return Object.fromEntries(
        Object.entries(mapValue).map(([key, item]) => [
          key,
          packMessageValue(mapValueMessage, item),
        ])
      );
    }
    default:
      return value;
  }
}

function packMessageValue(desc: DescMessage, value: unknown): unknown {
  if (desc.typeName !== ANY_TYPE_NAME) {
    return packAnyFields(desc, value);
  }

  // Already a packed Any object (has typeUrl) — pass through
  if (typeof value === 'object' && value !== null && 'typeUrl' in value) {
    return value;
  }

  if (typeof value === 'string') {
    return anyPack(StringValueSchema, create(StringValueSchema, { value }));
  }
  if (typeof value === 'boolean') {
    return anyPack(BoolValueSchema, create(BoolValueSchema, { value }));
  }
  if (typeof value === 'number') {
    if (Number.isInteger(value)) {
      return anyPack(Int64ValueSchema, create(Int64ValueSchema, { value: BigInt(value) }));
    }
    return anyPack(DoubleValueSchema, create(DoubleValueSchema, { value }));
  }

  // create() doesn't validate an Any's shape, so passing this through would silently produce
  // an empty Any (typeUrl: '') instead of a visible error.
  throw new Error(
    `packAnyFields: cannot auto-pack ${typeof value} into google.protobuf.Any — ` +
      `expected a string, number, or boolean primitive`
  );
}
