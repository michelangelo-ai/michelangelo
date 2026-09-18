import React from 'react';
import { useLocation, useNavigate } from 'react-router-dom-v5-compat';
import { useStyletron } from 'baseui';

import { CircleExclamationMark } from '#core/components/illustrations/circle-exclamation-mark/circle-exclamation-mark';
import { CircleExclamationMarkKind } from '#core/components/illustrations/circle-exclamation-mark/types';
import { Row } from '#core/components/row/row';
import { Signpost } from '#core/components/signpost/signpost';
import { DetailViewPageRenderer } from '#core/components/views/detail-view/components/detail-view-page-renderer/detail-view-page-renderer';
import { DetailViewPages } from '#core/components/views/detail-view/components/detail-view-pages/detail-view-pages';
import { RevisionSelector } from '#core/components/views/detail-view/components/revision-selector/revision-selector';
import { DetailView } from '#core/components/views/detail-view/detail-view';
import { PHASES } from '#core/config/phases/phases';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useInterpolationResolver } from '#core/interpolation/use-interpolation-resolver';
import { useEntityRecord } from './use-entity-record';

import type { PhaseConfig } from '#core/types/common/studio-types';

/**
 * Route component that handles entity detail views.
 *
 * Maps URL parameters to specific entity detail pages and handles:
 * - Entity not found scenarios
 * - Navigation back to entity list
 * - Revisioned entities, which render a Revision snapshot through the same detail config (see
 *   {@link useEntityRecord})
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

  const { record, loading, errorMessage } = useEntityRecord({
    service: entityConfig?.service ?? '',
    revisioned: !!entityConfig?.revisioned,
    projectId,
    entityId,
    revisionId,
  });

  // Tabs live in the path; the query string (e.g. `?revisionId=`) must survive tab changes.
  const handleTabNavigation = React.useCallback(
    (tabId: string, options?: { replace?: boolean }) => {
      navigate(`/${projectId}/${phase}/${entity}/${entityId}/${tabId}${search}`, options);
    },
    [navigate, projectId, phase, entity, entityId, search]
  );

  // Revision selection lives in the query string so the tab path is untouched.
  const handleRevisionSelect = (nextRevisionId: string) => {
    navigate({ pathname, search: `?revisionId=${encodeURIComponent(nextRevisionId)}` });
  };

  const handleReturnToEntityList = () => {
    navigate(`/${projectId}/${phase}/${entity}`);
  };

  // TODO: error handling for URLs that don't match any entity config
  const detailViewConfig =
    (entityConfig?.views ?? []).find((view) => view.type === 'detail') ?? undefined;

  React.useEffect(() => {
    if (errorMessage || loading) return;

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
  }, [entityTab, detailViewConfig, loading, errorMessage, handleTabNavigation]);

  if (errorMessage) {
    return (
      <Signpost
        title="Entity not found"
        description={`Could not load ${entity} "${entityId}". ${errorMessage}`}
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

  const resolvedDetailViewConfig = resolver(detailViewConfig, { page: record });
  return (
    <DetailView
      subtitle={entityConfig!.name}
      title={entityId}
      titleEnhancer={
        entityConfig!.revisioned && (
          <RevisionSelector
            service={entityConfig!.service}
            entityId={entityId}
            selectedRevisionId={revisionId}
            onSelect={handleRevisionSelect}
          />
        )
      }
      onGoBack={handleReturnToEntityList}
      actions={entityConfig!.actions}
      record={record}
      loading={loading}
      headerContent={
        <Row items={resolvedDetailViewConfig!.metadata} record={record} loading={loading} />
      }
    >
      <DetailViewPages
        tabs={resolvedDetailViewConfig!.pages.map((page) => ({
          id: page.id,
          label: page.label,
          content: <DetailViewPageRenderer page={page} data={record} isLoading={loading} />,
        }))}
        activeTabId={entityTab}
        onTabSelect={handleTabNavigation}
      />
    </DetailView>
  );
}
