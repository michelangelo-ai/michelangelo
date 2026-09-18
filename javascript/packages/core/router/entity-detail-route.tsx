import React from 'react';
import { useLocation, useNavigate } from 'react-router-dom-v5-compat';
import { useStyletron } from 'baseui';

import { CircleExclamationMark } from '#core/components/illustrations/circle-exclamation-mark/circle-exclamation-mark';
import { CircleExclamationMarkKind } from '#core/components/illustrations/circle-exclamation-mark/types';
import { Row } from '#core/components/row/row';
import { Signpost } from '#core/components/signpost/signpost';
import { DetailViewPageRenderer } from '#core/components/views/detail-view/components/detail-view-page-renderer/detail-view-page-renderer';
import { DetailViewPages } from '#core/components/views/detail-view/components/detail-view-pages/detail-view-pages';
import { DetailView } from '#core/components/views/detail-view/detail-view';
import { PHASES } from '#core/config/phases/phases';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { useInterpolationResolver } from '#core/interpolation/use-interpolation-resolver';
import { buildRevisionName } from '#core/utils/revision-utils';
import { capitalizeFirstLetter } from '#core/utils/string-utils';

import type { PhaseConfig } from '#core/types/common/studio-types';

/**
 * Route component that handles entity detail views.
 *
 * Maps URL parameters to specific entity detail pages and handles:
 * - Entity not found scenarios
 * - Navigation back to entity list
 * - Revision snapshots: for a `revisioned` entity, `?revisionId=` swaps the entity for the
 *   matching Revision's `spec.content`, rendered through the same detail view config. Without
 *   it the route resolves the entity's `status.latestRevision` and redirects to it.
 *
 * @param phases - Phase configuration override for testing. Defaults to {@link PHASES}.
 */
export function EntityDetailRoute({ phases = PHASES }: { phases?: Record<string, PhaseConfig> }) {
  const [, theme] = useStyletron();
  const { phase, entity, entityId, projectId, entityTab, revisionId } = useStudioParams('detail');
  const navigate = useNavigate();
  const { pathname, search } = useLocation();
  const entityConfig = phases[phase].entities.find((e) => e.id === entity);
  const resolver = useInterpolationResolver();

  const service = entityConfig?.service ?? '';
  const isRevisioned = !!entityConfig?.revisioned;
  const isRevisionView = !!revisionId && isRevisioned;

  const { data, isLoading, error } = useStudioQuery<Record<string, unknown>>({
    queryName: isRevisionView ? 'GetRevision' : `Get${capitalizeFirstLetter(service)}`,
    serviceOptions: {
      namespace: projectId,
      name: isRevisionView ? buildRevisionName(service, entityId, revisionId) : entityId,
    },
    clientOptions: {
      enabled: !!service && !!entityId,
    },
  });

  // cast: PhaseEntityConfig carries no entity type generic; only the revision pointer is read
  // from the live record here; see #1425
  const liveRecord = data?.[service] as
    | { status?: { latestRevision?: { name?: string } } }
    | undefined;
  const latestRevisionName = isRevisioned ? liveRecord?.status?.latestRevision?.name : undefined;
  const { data: latestRevisionData, isLoading: isLoadingLatestRevision } = useStudioQuery<{
    revision?: { spec?: { revisionId?: string } };
  }>({
    queryName: 'GetRevision',
    serviceOptions: { namespace: projectId, name: latestRevisionName ?? '' },
    clientOptions: { enabled: !isRevisionView && !!latestRevisionName },
  });
  const latestRevisionId = latestRevisionData?.revision?.spec?.revisionId;

  React.useEffect(() => {
    if (isRevisionView || !latestRevisionId) return;
    navigate(
      { pathname, search: `?revisionId=${encodeURIComponent(latestRevisionId)}` },
      { replace: true }
    );
  }, [isRevisionView, latestRevisionId, navigate, pathname]);

  const isResolvingLatestRevision =
    !isRevisionView && !!latestRevisionName && (isLoadingLatestRevision || !!latestRevisionId);
  const loading = isLoading || isResolvingLatestRevision;

  // Tabs live in the path; the query string (e.g. `?revisionId=`) must survive tab changes.
  const handleTabNavigation = React.useCallback(
    (tabId: string, options?: { replace?: boolean }) => {
      navigate(`/${projectId}/${phase}/${entity}/${entityId}/${tabId}${search}`, options);
    },
    [navigate, projectId, phase, entity, entityId, search]
  );

  const handleReturnToEntityList = () => {
    navigate(`/${projectId}/${phase}/${entity}`);
  };

  // TODO: error handling for URLs that don't match any entity config
  const detailViewConfig =
    (entityConfig?.views ?? []).find((view) => view.type === 'detail') ?? undefined;

  React.useEffect(() => {
    if (error || isLoading) return;

    if (!entityId || !detailViewConfig?.pages?.length) return;

    const validTabIds = detailViewConfig.pages.map((page) => page.id);
    const firstTabId = validTabIds[0];

    if (!entityTab) {
      // No tab specified - redirect to first tab
      handleTabNavigation(firstTabId, { replace: true });
    } else if (!validTabIds.includes(entityTab)) {
      // Invalid tab - redirect to first tab
      handleTabNavigation(firstTabId, { replace: true });
    }
  }, [entityTab, detailViewConfig, isLoading, error, handleTabNavigation]);

  if (error) {
    return (
      <Signpost
        title="Entity not found"
        description={`Could not load ${entity} "${entityId}". ${error.message}`}
        illustration={
          <CircleExclamationMark
            kind={CircleExclamationMarkKind.ERROR}
            width={theme.sizing.scale1600}
            height={theme.sizing.scale1600}
          />
        }
        buttonConfig={{
          onClick: () => navigate(`/${projectId}/${phase}/${entity}`),
          content: 'Back to list',
        }}
      />
    );
  }

  // cast: the Revision response is untyped here; the route only reads the wrapped entity at
  // spec.content, which the RPC layer unpacks into a plain object; see #1425
  const revision = data?.revision as { spec?: { content?: Record<string, unknown> } } | undefined;
  // cast: PhaseEntityConfig carries no entity type generic, so the runtime-selected service key's
  // value is assumed to be the entity object; related to #1425. In a revision view the snapshot
  // is the same entity shape, so the same detail config reads from it unchanged.
  const entityData = (isRevisionView ? revision?.spec?.content : data?.[service]) as
    | Record<string, unknown>
    | undefined;
  const resolvedDetailViewConfig = resolver(detailViewConfig, { page: entityData });
  return (
    <DetailView
      subtitle={entityConfig!.name}
      title={entityId}
      onGoBack={handleReturnToEntityList}
      actions={entityConfig!.actions}
      record={entityData}
      revision={
        isRevisionView
          ? { name: buildRevisionName(service, entityId, revisionId), namespace: projectId }
          : undefined
      }
      loading={loading}
      headerContent={
        <Row items={resolvedDetailViewConfig!.metadata} record={entityData} loading={loading} />
      }
    >
      <DetailViewPages
        tabs={resolvedDetailViewConfig!.pages.map((page) => ({
          id: page.id,
          label: page.label,
          content: <DetailViewPageRenderer page={page} data={entityData} isLoading={loading} />,
        }))}
        activeTabId={entityTab}
        onTabSelect={handleTabNavigation}
      />
    </DetailView>
  );
}
