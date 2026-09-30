import { ObjectMetaSchema } from './gen/k8s.io/apimachinery/pkg/apis/meta/v1/generated_pb';
import { walkMessage } from './walk-message';

import type { DescMessage, Registry } from '@bufbuild/protobuf';

/**
 * Removes `deletionGracePeriodSeconds` from every Kubernetes object metadata in a request,
 * including metadata inside Any payloads (e.g. Revision content).
 *
 * Envoy prints default values into responses (`always_print_primitive_fields`), and that
 * includes this unset proto2 field as `"0"`. Sending a fetched record back on update would
 * then set it explicitly, which the API server rejects as a change to an immutable field.
 * The server owns this field, so requests never need to carry it.
 */
export function omitDeletionGracePeriod(
  desc: DescMessage,
  value: unknown,
  registry: Registry
): unknown {
  return walkMessage(
    desc,
    value,
    (message, item, descend) => {
      if (message.typeName !== ObjectMetaSchema.typeName) return descend(message, item);
      if (item === null || typeof item !== 'object') return item;
      // cast: object metadata in proto3 JSON is a plain object
      const { deletionGracePeriodSeconds: _omitted, ...rest } = item as Record<string, unknown>;
      return rest;
    },
    registry
  );
}
