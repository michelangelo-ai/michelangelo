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
 * While that lookup is loading or has failed, `revision` is undefined for a reason a run must
 * not proceed past: callers pass the result to `assertRevisionResolved` before submitting so a
 * run is never created unpinned because of a slow or failed lookup.
 */
export function useTargetRevision(record: Pipeline | PipelineRevision | undefined): {
  revision: RevisionRef | undefined;
  isLoading: boolean;
  isError: boolean;
  errorMessage: string | undefined;
} {
  const revisionRecord = isPipelineRevision(record) ? record : undefined;
  const pointerRef = revisionRecord ? undefined : getLatestRevisionRef(record);
  const needsFallback = !!record && !revisionRecord && !pointerRef;

  const fallbackQuery = useStudioQuery<{
    revisionList?: { items?: { metadata: { name: string; namespace: string } }[] };
  }>({
    queryName: 'ListRevision',
    serviceOptions: {
      namespace: record?.metadata?.namespace,
      listOptionsExt: buildRevisionListOptions('pipeline', record?.metadata?.name ?? ''),
    },
    clientOptions: { enabled: needsFallback },
  });

  if (revisionRecord) {
    return {
      revision: {
        name: revisionRecord.metadata.name,
        namespace: revisionRecord.metadata.namespace,
      },
      isLoading: false,
      isError: false,
      errorMessage: undefined,
    };
  }
  if (pointerRef)
    return { revision: pointerRef, isLoading: false, isError: false, errorMessage: undefined };

  const latest = needsFallback ? fallbackQuery.data?.revisionList?.items?.[0] : undefined;
  return {
    revision: latest
      ? { name: latest.metadata.name, namespace: latest.metadata.namespace }
      : undefined,
    // isFetching, not isLoading: cached data can be stale while a background refetch runs.
    isLoading: needsFallback && fallbackQuery.isFetching,
    isError: needsFallback && fallbackQuery.isError,
    errorMessage: needsFallback ? fallbackQuery.error?.message : undefined,
  };
}

/**
 * Throws while the target revision lookup is loading or failed. `FormDialog` renders errors
 * thrown from `onSubmit` in the dialog, so the user sees why Run did nothing.
 */
export function assertRevisionResolved({
  isLoading,
  isError,
  errorMessage,
}: {
  isLoading: boolean;
  isError: boolean;
  errorMessage: string | undefined;
}): void {
  if (isError) {
    throw new Error(`Could not load the pipeline's latest revision. ${errorMessage ?? ''}`.trim());
  }
  if (isLoading) {
    throw new Error("Still loading the pipeline's latest revision. Try again in a moment.");
  }
}
