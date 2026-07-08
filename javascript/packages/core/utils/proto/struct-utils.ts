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
  const pairs = Object.entries(value);
  const [kind, kindValue] = pairs[0] || [];
  // cast: Object.entries returns string keys; kind is always one of the STRUCT_KINDS keys in a
  // valid protobuf value
  return { kind: kind as keyof typeof STRUCT_KINDS, kindValue };
}

/**
 * Decodes protocol buffer struct fields to JavaScript objects
 */
function decodeStructFields(fields: Fields): Record<string, unknown> {
  const result: Record<string, unknown> = {};

  for (const [key, value] of Object.entries(fields)) {
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
    // cast: struct passed the typeof object check; ProtobufValue is the expected shape for protobuf
    // value objects
    const value = struct as ProtobufValue;
    const { kind, kindValue } = getDistinctKindValue(value);

    switch (kind) {
      case STRUCT_KINDS.listValue:
        if (kindValue && typeof kindValue === 'object' && 'values' in kindValue) {
          // cast: kindValue passed the object + 'values' checks; narrowing to the listValue proto
          // shape
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
