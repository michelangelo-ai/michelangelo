import type { DescField, DescMessage } from '@bufbuild/protobuf';
import type { MessageVisitor } from './types';

/**
 * Copies a JSON object shaped like `desc`, passing every nested message value (singular,
 * repeated, or map) through `visit`. Keys that aren't fields of `desc` are copied unchanged.
 *
 * @example
 * // Replace every Any in a request; walk into all other messages
 * walkMessage(ListPipelineRunRequestSchema, request, (desc, value, descend) =>
 *   desc.typeName === 'google.protobuf.Any' ? packAny(value) : descend(desc, value)
 * );
 */
export function walkMessage(desc: DescMessage, value: unknown, visit: MessageVisitor): unknown {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return value;

  const descend = (message: DescMessage, item: unknown) => walkMessage(message, item, visit);
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
