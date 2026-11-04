import { useStudioQuery } from '#core/hooks/use-studio-query';
import { buildRevisionListOptions, buildRevisionName } from '#core/utils/revision-utils';
import { capitalizeFirstLetter } from '#core/utils/string-utils';

/**
 * Loads the record an entity detail page renders.
 *
 * A plain entity is fetched and rendered as-is. A `revisioned` entity always renders the
 * Revision itself instead — not just its wrapped `spec.content` — so configs can read the
 * Revision's own identity (e.g. `metadata.name`) the same way list rows for that entity already
 * do: the Revision `?revisionId=` names, or else the one the entity's `status.latestRevision`
 * points at. The controller only sets that pointer for default-branch (`main`/`master`)
 * commits, so an entity with none falls back to its most recently updated Revision. The live
 * entity is always loaded for a revisioned entity, even when the URL names the Revision, so the
 * page can tell which Revision is the latest.
 */
export function useEntityRecord({
  service,
  revisioned,
  projectId,
  entityId,
  revisionId,
}: {
  service: string;
  revisioned: boolean;
  projectId: string;
  entityId: string;
  revisionId?: string;
}): {
  record: Record<string, unknown> | undefined;
  /** Name of the Revision the entity's `status.latestRevision` points at, if revisioned. */
  latestRevisionName: string | undefined;
  loading: boolean;
  errorMessage: string | undefined;
} {
  const revisionNameFromUrl =
    revisioned && revisionId ? buildRevisionName(service, entityId, revisionId) : undefined;

  // The live entity: the record itself for a plain entity, or the pointer to the latest
  // Revision for a revisioned one.
  const entityQuery = useStudioQuery<Record<string, unknown>>({
    queryName: `Get${capitalizeFirstLetter(service)}`,
    serviceOptions: { namespace: projectId, name: entityId },
    clientOptions: { enabled: !!service },
  });

  // cast: PhaseEntityConfig carries no entity type generic, so the runtime-selected service
  // key's value is assumed to be the entity object; see #1425
  const entityRecord = entityQuery.data?.[service] as Record<string, unknown> | undefined;
  // cast: only an entity that opted into `revisioned` is expected to carry the controller-written
  // `status.latestRevision` pointer, so it is read solely behind that flag below; see #1425
  const revisionedRecord = entityRecord as
    | { status?: { latestRevision?: { name?: string } } }
    | undefined;
  const revisionNameFromEntity = revisioned
    ? revisionedRecord?.status?.latestRevision?.name
    : undefined;

  // No explicit or pointed-at Revision: the entity has no default-branch Revision, so look up
  // its Revisions and render the most recently updated one rather than erroring.
  const needsFallbackRevision =
    revisioned && !!entityQuery.data && !revisionNameFromUrl && !revisionNameFromEntity;
  const fallbackQuery = useStudioQuery<{
    revisionList?: { items?: { metadata: { name: string } }[] };
  }>({
    queryName: 'ListRevision',
    serviceOptions: {
      namespace: projectId,
      listOptionsExt: buildRevisionListOptions(service, entityId),
    },
    clientOptions: { enabled: needsFallbackRevision },
  });
  // The revision selector issues this same request, so the query's state is only this hook's
  // concern while the fallback is actually in use.
  const fallbackRevisionName = needsFallbackRevision
    ? fallbackQuery.data?.revisionList?.items?.[0]?.metadata.name
    : undefined;
  const fallbackLoading = needsFallbackRevision && fallbackQuery.isLoading;
  const fallbackError = needsFallbackRevision ? fallbackQuery.error?.message : undefined;

  const revisionName = revisionNameFromUrl ?? revisionNameFromEntity ?? fallbackRevisionName;
  const revisionQuery = useStudioQuery<{ revision?: Record<string, unknown> }>({
    queryName: 'GetRevision',
    serviceOptions: { namespace: projectId, name: revisionName },
    clientOptions: { enabled: !!revisionName },
  });

  // The fallback lookup finished and found no Revision at all, so the revision query was never
  // enabled and no query errors. A revisioned entity has nothing to render in that state.
  const hasNoRevisionToRender =
    needsFallbackRevision && !!fallbackQuery.data && !fallbackRevisionName;

  return {
    record: revisioned ? revisionQuery.data?.revision : entityRecord,
    latestRevisionName: revisionNameFromEntity,
    loading: entityQuery.isLoading || fallbackLoading || revisionQuery.isLoading,
    errorMessage:
      entityQuery.error?.message ??
      fallbackError ??
      revisionQuery.error?.message ??
      (hasNoRevisionToRender ? 'No revision found.' : undefined),
  };
}
