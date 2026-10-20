import {
  BoolValueSchema,
  DoubleValueSchema,
  Int64ValueSchema,
  StringValueSchema,
} from '@bufbuild/protobuf/wkt';

import { walkMessage } from './walk-message';

import type { DescMessage } from '@bufbuild/protobuf';

const ANY_TYPE_NAME = 'google.protobuf.Any';
const TYPE_URL_PREFIX = 'type.googleapis.com/';

/**
 * Walks a proto3 JSON request object against its proto descriptor, packing JS primitives into
 * the JSON form of well-known wrapper types wherever the schema has a `google.protobuf.Any` field.
 *
 * @example
 * packAnyFields(CriterionSchema, { fieldName: "x", matchValue: "my-pipeline" })
 * // matchValue -> { "@type": "type.googleapis.com/google.protobuf.StringValue", value: "my-pipeline" }
 */
export function packAnyFields(desc: DescMessage, value: unknown): unknown {
  return walkMessage(desc, value, (message, item, descend) =>
    message.typeName === ANY_TYPE_NAME ? packAny(item) : descend(message, item)
  );
}

function packAny(value: unknown): unknown {
  // Already a proto3 JSON Any (has @type) — pass through
  if (typeof value === 'object' && value !== null && '@type' in value) {
    return value;
  }

  if (typeof value === 'string') {
    return packWrapper(StringValueSchema, value);
  }
  if (typeof value === 'boolean') {
    return packWrapper(BoolValueSchema, value);
  }
  if (typeof value === 'number') {
    if (Number.isInteger(value)) {
      // proto3 JSON encodes int64 as a string
      return packWrapper(Int64ValueSchema, String(value));
    }
    return packWrapper(DoubleValueSchema, value);
  }

  throw new Error(
    `packAnyFields: cannot auto-pack ${typeof value} into google.protobuf.Any — ` +
      `expected a string, number, or boolean primitive`
  );
}

function packWrapper(schema: DescMessage, value: unknown) {
  return { '@type': `${TYPE_URL_PREFIX}${schema.typeName}`, value };
}
