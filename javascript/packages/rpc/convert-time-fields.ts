import { create, fromJson, toJson } from '@bufbuild/protobuf';
import { DurationSchema, TimestampSchema } from '@bufbuild/protobuf/wkt';

import type { DescField, DescMessage, Registry } from '@bufbuild/protobuf';
import type { ConvertTime } from './types';

/**
 * Converts every google.protobuf.Timestamp/Duration field in a response from its proto3 JSON
 * string form ("2026-09-28T23:11:19.992Z", "3600s") to `{ seconds, nanos }`, so all time fields
 * reach the UI in one shape. Envoy's transcoder always prints these types as strings.
 */
export function timesToObjects(desc: DescMessage, value: unknown, registry: Registry): unknown {
  return walkMessage(desc, value, registry, (schema, time) => {
    if (typeof time !== 'string') return time;
    const message = fromJson(schema, time);
    return { seconds: message.seconds.toString(), nanos: message.nanos };
  });
}

/** Inverse of {@link timesToObjects}, applied to requests before they're parsed as proto3 JSON. */
export function timesToStrings(desc: DescMessage, value: unknown, registry: Registry): unknown {
  return walkMessage(desc, value, registry, (schema, time) => {
    if (time === null || typeof time !== 'object') return time;
    // cast: a non-string time value is the { seconds, nanos } shape timesToObjects produces
    const { seconds = 0, nanos = 0 } = time as Partial<Record<'seconds' | 'nanos', unknown>>;
    return toJson(
      schema,
      create(schema, { seconds: BigInt(String(seconds)), nanos: Number(nanos) })
    );
  });
}

function walkMessage(
  desc: DescMessage,
  value: unknown,
  registry: Registry,
  convert: ConvertTime
): unknown {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return value;

  // An Any's payload fields sit beside its @type; walk them as the payload message.
  if (desc.typeName === 'google.protobuf.Any') {
    // cast: an Any in proto3 JSON is an object whose @type names the payload message
    const typeUrl = (value as { '@type'?: unknown })['@type'];
    const payload =
      typeof typeUrl === 'string' ? registry.getMessage(typeUrl.replace(/^.*\//, '')) : undefined;
    return payload ? walkMessage(payload, value, registry, convert) : value;
  }

  const result: Record<string, unknown> = {};
  for (const [key, val] of Object.entries(value)) {
    const field: DescField | undefined = desc.field[key];
    result[key] = field ? walkField(field, val, registry, convert) : val;
  }
  return result;
}

function walkField(
  field: DescField,
  value: unknown,
  registry: Registry,
  convert: ConvertTime
): unknown {
  if (value === null || value === undefined) return value;

  switch (field.fieldKind) {
    case 'message':
      return walkMessageValue(field.message, value, registry, convert);
    case 'list':
      if (field.listKind !== 'message') return value;
      // cast: repeated fields are always arrays at runtime
      return (value as unknown[]).map((item) =>
        walkMessageValue(field.message, item, registry, convert)
      );
    case 'map': {
      const mapValueMessage = field.message;
      if (!mapValueMessage) return value;
      return Object.fromEntries(
        // cast: map fields are always plain objects at runtime
        Object.entries(value as Record<string, unknown>).map(([key, item]) => [
          key,
          walkMessageValue(mapValueMessage, item, registry, convert),
        ])
      );
    }
    default:
      return value;
  }
}

function walkMessageValue(
  desc: DescMessage,
  value: unknown,
  registry: Registry,
  convert: ConvertTime
): unknown {
  if (desc.typeName === TimestampSchema.typeName) return convert(TimestampSchema, value);
  if (desc.typeName === DurationSchema.typeName) return convert(DurationSchema, value);
  return walkMessage(desc, value, registry, convert);
}
