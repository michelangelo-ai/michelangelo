/**
 * Forms accepted by timestamp helpers such as `toEpochSeconds`:
 * - epoch seconds as a number or numeric string
 * - RFC 3339 string (proto3 JSON Timestamp)
 * - a `{ seconds, nanos }` protobuf Timestamp object
 *   (`nanos` is ignored; only whole seconds are used;
 *   `seconds` may be `bigint` from native protobuf-es Timestamps)
 */
export type TimestampInput =
  | string
  | number
  | { seconds?: string | number | bigint | null; nanos?: string | number | null };
