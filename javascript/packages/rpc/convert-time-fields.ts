import { create, fromJson, toJson } from '@bufbuild/protobuf';
import { DurationSchema, TimestampSchema } from '@bufbuild/protobuf/wkt';

import { walkMessage } from './walk-message';

import type { DescMessage, Registry } from '@bufbuild/protobuf';
import type { ConvertTime } from './types';

/**
 * Converts every google.protobuf.Timestamp/Duration field in a response from its proto3 JSON
 * string ("2026-09-28T23:11:19.992Z", "3600s") to `{ seconds, nanos }`, the shape the UI's time
 * handling reads.
 */
export function timesToObjects(desc: DescMessage, value: unknown, registry: Registry): unknown {
  return convertTimes(desc, value, registry, (schema, time) => {
    if (typeof time !== 'string') return time;
    const message = fromJson(schema, time);
    return { seconds: message.seconds.toString(), nanos: message.nanos };
  });
}

/** Inverse of {@link timesToObjects}, applied to requests before they're parsed as proto3 JSON. */
export function timesToStrings(desc: DescMessage, value: unknown, registry: Registry): unknown {
  return convertTimes(desc, value, registry, (schema, time) => {
    if (time === null || typeof time !== 'object') return time;
    // cast: a non-string time value is the { seconds, nanos } shape timesToObjects produces
    const { seconds = 0, nanos = 0 } = time as Partial<Record<'seconds' | 'nanos', unknown>>;
    return toJson(
      schema,
      create(schema, { seconds: BigInt(String(seconds)), nanos: Number(nanos) })
    );
  });
}

function convertTimes(
  desc: DescMessage,
  value: unknown,
  registry: Registry,
  convert: ConvertTime
): unknown {
  return walkMessage(
    desc,
    value,
    (message, item, descend) => {
      if (message.typeName === TimestampSchema.typeName) return convert(TimestampSchema, item);
      if (message.typeName === DurationSchema.typeName) return convert(DurationSchema, item);
      return descend(message, item);
    },
    registry
  );
}
