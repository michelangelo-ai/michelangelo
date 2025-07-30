import { toPairs } from 'lodash';

import { STRUCT_KINDS } from './constants';

import type { DecodedStruct, Fields, ProtobufValue, Struct } from './types';

/**
 * Type guard to check if a value is a protocol buffer struct
 */
export function isStruct(struct: unknown): struct is Struct {
  return (
    !!struct &&
    typeof struct === 'object' &&
    'fields' in struct &&
    typeof struct.fields === 'object' &&
    !Array.isArray(struct.fields)
  );
}

/**
 * Helper function to get the distinct kind value from a protobuf value
 */
function getDistinctKindValue(value: ProtobufValue): {
  kind: keyof typeof STRUCT_KINDS;
  kindValue: unknown;
} {
  const pairs = toPairs(value);
  const [kind, kindValue] = pairs[0] || [];
  return { kind: kind as keyof typeof STRUCT_KINDS, kindValue };
}

/**
 * Decodes protocol buffer struct fields to JavaScript objects
 */
function decodeStructFields(fields: Fields): Record<string, unknown> {
  const result: Record<string, unknown> = {};

  for (const [key, value] of toPairs(fields)) {
    result[key] = decodeStruct(value);
  }

  return result;
}

/**
 * Decodes protocol buffer structs to JavaScript objects
 */
export function decodeStruct(struct: unknown): DecodedStruct {
  // Handle null/undefined
  if (struct == null) {
    return struct;
  }

  // Handle direct struct with fields
  if (isStruct(struct)) {
    return decodeStructFields(struct.fields);
  }

  // Handle value objects
  if (typeof struct === 'object' && struct !== null) {
    const value = struct as ProtobufValue;
    const { kind, kindValue } = getDistinctKindValue(value);

    switch (kind) {
      case STRUCT_KINDS.listValue:
        if (kindValue && typeof kindValue === 'object' && 'values' in kindValue) {
          const listValue = kindValue as { values: ProtobufValue[] };
          return listValue.values.map((item) => decodeStruct(item));
        }
        return [];

      case STRUCT_KINDS.nullValue:
        return null;

      case STRUCT_KINDS.structValue:
        if (kindValue) {
          return decodeStruct(kindValue);
        }
        return {};

      case STRUCT_KINDS.boolValue:
      case STRUCT_KINDS.numberValue:
      case STRUCT_KINDS.stringValue:
        return kindValue;

      default:
        return struct;
    }
  }

  return struct;
}
