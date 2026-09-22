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
