import { capitalizeFirstLetter } from '#core/utils/string-utils';

import type { RevisionRef } from '#core/types/common/studio-types';

/**
 * Revision ids are git refs. To follow the controller naming scheme for
 * revisioned entity, we display the first 12 characters of the git ref.
 */
export const REVISION_ID_DISPLAY_LENGTH = 12;

/**
 * Builds human-readable label and truncates the revisionId by REVISION_ID_DISPLAY_LENGTH
 */
export function formatRevisionLabel(revisionId?: string): string {
  return revisionId ? `Revision ${revisionId.slice(0, REVISION_ID_DISPLAY_LENGTH)}` : '';
}

/**
 * Generates the name of the Revision CR snapshotting a revisioned entity.
 */
export function buildRevisionName(service: string, entityId: string, revisionId: string): string {
  switch (service) {
    case 'pipeline':
      return `pipeline-${entityId.toLowerCase()}-${revisionId.slice(0, REVISION_ID_DISPLAY_LENGTH)}`;
    default:
      return entityId;
  }
}

/**
 * The `status.latestRevision` pointer of a revisioned entity, or undefined when the entity has
 * none yet (or isn't revisioned). Both fields are required on the wire when the pointer is set;
 * this narrows the loosely-typed record to a usable {@link RevisionRef}.
 */
export function getLatestRevisionRef(record: unknown): RevisionRef | undefined {
  // cast: table rows are untyped; only the latestRevision pointer is read, and it is
  // narrowed below before use; see #1425
  const latest = (record as { status?: { latestRevision?: { name?: string; namespace?: string } } })
    ?.status?.latestRevision;
  return latest?.name && latest.namespace
    ? { name: latest.name, namespace: latest.namespace }
    : undefined;
}

/** `SORT_ORDER_DESC` from `proto/api/list.proto` — protobuf-es encodes enums by number. */
const SORT_ORDER_DESC = 2;

/** `CRITERION_OPERATOR_EQUAL` from `proto/api/list.proto`. */
const CRITERION_OPERATOR_EQUAL = 1;

/**
 * `ListRevision` options selecting every Revision of one entity, most recently updated first.
 * Shared so the detail page's fallback lookup and the revision selector issue the same request
 * and hit the same query cache entry. Pass `limit` when only the newest few are needed.
 */
export function buildRevisionListOptions(service: string, entityId: string, limit?: number) {
  return {
    operation: {
      criterion: [
        {
          fieldName: 'revision.base_type',
          operator: CRITERION_OPERATOR_EQUAL,
          matchValue: capitalizeFirstLetter(service),
        },
        {
          fieldName: 'revision.base_resource_name',
          operator: CRITERION_OPERATOR_EQUAL,
          matchValue: entityId,
        },
      ],
    },
    orderBy: [{ field: 'metadata.update_timestamp', dir: SORT_ORDER_DESC }],
    ...(limit && { pagination: { offset: 0, limit } }),
  };
}
