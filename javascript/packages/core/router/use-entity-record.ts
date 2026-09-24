import { useStudioQuery } from '#core/hooks/use-studio-query';
import { buildRevisionName } from '#core/utils/revision-utils';
import { capitalizeFirstLetter } from '#core/utils/string-utils';

/**
 * Loads the record an entity detail page renders.
 *
 * A plain entity is fetched and rendered as-is. A `revisioned` entity always renders the
 * Revision itself instead — not just its wrapped `spec.content` — so configs can read the
 * Revision's own identity (e.g. `metadata.name`) the same way list rows for that entity already
 * do: the Revision `?revisionId=` names, or else the one the entity's `status.latestRevision`
 * points at.
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
  loading: boolean;
  errorMessage: string | undefined;
} {
  const revisionNameFromUrl =
    revisioned && revisionId ? buildRevisionName(service, entityId, revisionId) : undefined;

  // The live entity: the record itself for a plain entity, or the pointer to the latest
  // Revision for a revisioned one. Not needed when the URL already names the Revision.
  const entityQuery = useStudioQuery<Record<string, unknown>>({
    queryName: `Get${capitalizeFirstLetter(service)}`,
    serviceOptions: { namespace: projectId, name: entityId },
    clientOptions: { enabled: !!service && !revisionNameFromUrl },
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

  const revisionName = revisionNameFromUrl ?? revisionNameFromEntity;
  const revisionQuery = useStudioQuery<{ revision?: Record<string, unknown> }>({
    queryName: 'GetRevision',
    serviceOptions: { namespace: projectId, name: revisionName },
    clientOptions: { enabled: !!revisionName },
  });

  // The entity loaded but carries no `status.latestRevision`, so the revision query was never
  // enabled and neither query errors. A revisioned entity has nothing to render in that state.
  const hasNoRevisionToRender = revisioned && !!entityQuery.data && !revisionName;

  return {
    record: revisioned ? revisionQuery.data?.revision : entityRecord,
    loading: entityQuery.isLoading || revisionQuery.isLoading,
    errorMessage:
      entityQuery.error?.message ??
      revisionQuery.error?.message ??
      (hasNoRevisionToRender ? 'No revision found.' : undefined),
  };
}
