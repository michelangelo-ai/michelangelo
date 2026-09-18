/**
 * Revision ids are full git refs; the leading 12 characters are enough to identify one. The
 * pipeline controller uses the same prefix when naming the Revision CR it snapshots
 * (`formatRevisionName` in go/components/pipeline/controller.go).
 */
export const REVISION_ID_DISPLAY_LENGTH = 12;

/** Human-readable label for a revision id, e.g. `Revision 3f2a1b9c0d4e`. */
export function formatRevisionId(revisionId?: string): string {
  return revisionId ? `Revision ${revisionId.slice(0, REVISION_ID_DISPLAY_LENGTH)}` : '';
}

/**
 * Derives the name of the Revision CR snapshotting `entityId` at `revisionId`.
 *
 * Mirrors the controller's naming scheme (`<kind>-<lower(name)>-<revisionId[:12]>`) so a detail
 * view can `GetRevision` directly from the `?revisionId=` query param — `revision_id` is not an
 * indexed column, so it cannot be filtered server-side through `ListRevision`.
 *
 * @param service - The revisioned entity's service name, e.g. `pipeline`
 */
export function buildRevisionName(service: string, entityId: string, revisionId: string): string {
  return `${service}-${entityId.toLowerCase()}-${revisionId.slice(0, REVISION_ID_DISPLAY_LENGTH)}`;
}
