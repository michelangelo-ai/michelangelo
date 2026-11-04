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
 *
 * `isLoading` covers background refetches too, so cached data never pins a stale revision.
 *
 * While that lookup is loading or has failed, `revision` is undefined for a reason a run must
 * not proceed past: callers pass the result to `getSubmitDisabledReason` and disable submit,
 * so a run is never created unpinned because of a slow or failed lookup.
 */
export function useTargetRevision(record: Pipeline | PipelineRevision | undefined): {
  revision: RevisionRef | undefined;
  isLoading: boolean;
  isError: boolean;
  errorMessage: string | undefined;
} {
  const revisionRef: RevisionRef | undefined = isPipelineRevision(record)
    ? { name: record.metadata.name, namespace: record.metadata.namespace }
    : getLatestRevisionRef(record);
  const needsFallback = !!record && !revisionRef;

  const fallbackQuery = useStudioQuery<{
    revisionList?: { items?: { metadata: { name: string; namespace: string } }[] };
  }>({
    queryName: 'ListRevision',
    serviceOptions: {
      namespace: record?.metadata?.namespace,
      listOptionsExt: buildRevisionListOptions('pipeline', record?.metadata?.name ?? '', 1),
    },
    clientOptions: { enabled: needsFallback },
  });

  const latest = needsFallback ? fallbackQuery.data?.revisionList?.items?.[0] : undefined;
  return {
    revision:
      revisionRef ??
      (latest ? { name: latest.metadata.name, namespace: latest.metadata.namespace } : undefined),
    isLoading: needsFallback && fallbackQuery.isFetching,
    isError: needsFallback && fallbackQuery.isError,
    errorMessage: needsFallback ? fallbackQuery.error?.message : undefined,
  };
}

/**
 * Why submit must stay disabled while the target revision lookup is loading or failed, or
 * undefined once a run can go ahead. Shown as the submit button's tooltip.
 */
export function getSubmitDisabledReason({
  isLoading,
  isError,
  errorMessage,
}: ReturnType<typeof useTargetRevision>): string | undefined {
  if (isError) {
    return `Could not load the pipeline's latest revision. ${errorMessage ?? ''}`.trim();
  }
  if (isLoading) {
    return "Loading the pipeline's latest revision…";
  }
  return undefined;
}
