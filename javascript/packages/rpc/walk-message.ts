import type { DescField, DescMessage, Registry } from '@bufbuild/protobuf';
import type { MessageVisitor } from './types';

const ANY_TYPE_NAME = 'google.protobuf.Any';

/**
 * Copies a JSON object shaped like `desc`, passing every nested message value (singular,
 * repeated, or map) through `visit`. Keys that aren't fields of `desc` are copied unchanged.
 *
 * With a `registry`, descending into a proto3 JSON `Any` walks its payload fields as the
 * message its `@type` names. An `Any` whose type isn't in the registry is copied unchanged.
 *
 * @example
 * // Replace every Any in a request; walk into all other messages
 * walkMessage(ListPipelineRunRequestSchema, request, (desc, value, descend) =>
 *   desc.typeName === 'google.protobuf.Any' ? packAny(value) : descend(desc, value)
 * );
 */
export function walkMessage(
  desc: DescMessage,
  value: unknown,
  visit: MessageVisitor,
  registry?: Registry
): unknown {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return value;

  if (registry && desc.typeName === ANY_TYPE_NAME) {
    // cast: an Any in proto3 JSON is an object whose @type names the payload message
    const typeUrl = (value as { '@type'?: unknown })['@type'];
    const payload =
      typeof typeUrl === 'string' ? registry.getMessage(typeUrl.replace(/^.*\//, '')) : undefined;
    return payload ? walkMessage(payload, value, visit, registry) : value;
  }

  const descend = (message: DescMessage, item: unknown) =>
    walkMessage(message, item, visit, registry);
  const visitValue = (message: DescMessage, item: unknown) => visit(message, item, descend);

  const result: Record<string, unknown> = {};
  for (const [key, val] of Object.entries(value)) {
    const field: DescField | undefined = desc.field[key];
    result[key] = field ? walkField(field, val, visitValue) : val;
  }
  return result;
}

function walkField(
  field: DescField,
  value: unknown,
  visitValue: (message: DescMessage, item: unknown) => unknown
): unknown {
  if (value === null || value === undefined) return value;

  switch (field.fieldKind) {
    case 'message':
      return visitValue(field.message, value);
    case 'list':
      if (field.listKind !== 'message') return value;
      // cast: repeated fields are always arrays at runtime
      return (value as unknown[]).map((item) => visitValue(field.message, item));
    case 'map': {
      const mapValueMessage = field.message;
      if (!mapValueMessage) return value;
      return Object.fromEntries(
        // cast: map fields are always plain objects at runtime, keyed by the (stringified)
        // map key regardless of its declared scalar type
        Object.entries(value as Record<string, unknown>).map(([key, item]) => [
          key,
          visitValue(mapValueMessage, item),
        ])
      );
    }
    default:
      return value;
  }
}
