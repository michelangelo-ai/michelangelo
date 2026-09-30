import { create } from '@bufbuild/protobuf';
import {
  anyPack,
  BoolValueSchema,
  DoubleValueSchema,
  Int64ValueSchema,
  StringValueSchema,
} from '@bufbuild/protobuf/wkt';

import { walkMessage } from './walk-message';

import type { DescMessage } from '@bufbuild/protobuf';

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
  return walkMessage(desc, value, (message, item, descend) =>
    message.typeName === ANY_TYPE_NAME ? packAny(item) : descend(message, item)
  );
}

function packAny(value: unknown): unknown {
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
