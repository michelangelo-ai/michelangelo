import { useStudioQuery } from '#core/hooks/use-studio-query';
import { buildRevisionListOptions, getLatestRevisionRef } from '#core/utils/revision-utils';
import { isPipelineRevision } from './types';

import type { RevisionRef } from '#core/types/common/studio-types';
import type { Pipeline, PipelineRevision } from './types';

/**
 * The Revision a run started from this record should pin to.
 *
 * A Revision record already names the exact revision being viewed — its own identity,
 * no lookup needed. A Pipeline record has no specific revision in view, so this falls
 * back to its `status.latestRevision` pointer. The controller only sets that pointer for
 * default-branch commits, so a Pipeline without one falls back to its most recently
 * updated Revision, matching the detail page.
 */
export function useTargetRevision(
  record: Pipeline | PipelineRevision | undefined
): RevisionRef | undefined {
  const revisionRecord = isPipelineRevision(record) ? record : undefined;
  const pointerRef = revisionRecord ? undefined : getLatestRevisionRef(record);
  const needsFallback = !!record && !revisionRecord && !pointerRef;

  const fallbackQuery = useStudioQuery<{
    revisionList?: { items?: { metadata: { name: string; namespace: string } }[] };
  }>({
    queryName: 'ListRevision',
    serviceOptions: {
      namespace: record?.metadata.namespace,
      listOptionsExt: buildRevisionListOptions('pipeline', record?.metadata.name ?? ''),
    },
    clientOptions: { enabled: needsFallback },
  });

  if (revisionRecord) {
    return { name: revisionRecord.metadata.name, namespace: revisionRecord.metadata.namespace };
  }
  if (pointerRef) return pointerRef;

  const latest = needsFallback ? fallbackQuery.data?.revisionList?.items?.[0] : undefined;
  return latest ? { name: latest.metadata.name, namespace: latest.metadata.namespace } : undefined;
}
