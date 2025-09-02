import { useNavigate } from 'react-router-dom-v5-compat';
import { useStyletron } from 'baseui';

import { ErrorView } from '#core/components/error-view/error-view';
import { CircleExclamationMark } from '#core/components/illustrations/circle-exclamation-mark/circle-exclamation-mark';
import { CircleExclamationMarkKind } from '#core/components/illustrations/circle-exclamation-mark/types';
import { Row } from '#core/components/row/row';
import { DetailView } from '#core/components/views/detail-view/detail-view';
import { PHASES } from '#core/config/phases/phases';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { capitalizeFirstLetter } from '#core/utils/string-utils';

import type { PhaseConfig } from '#core/types/common/studio-types';

/**
 * Route component that handles entity detail views.
 *
 * Maps URL parameters to specific entity detail pages and handles:
 * - Entity not found scenarios
 * - Navigation back to entity list
 *
 * @param phases - Phase configuration override for testing. Defaults to {@link PHASES}.
 */
export function EntityDetailRoute({ phases = PHASES }: { phases?: Record<string, PhaseConfig> }) {
  const [, theme] = useStyletron();
  const { phase, entity, entityId, projectId } = useStudioParams('detail');
  const navigate = useNavigate();
  const entityConfig = phases[phase].entities.find((e) => e.id === entity);
  const { data, isLoading, error } = useStudioQuery<Record<string, unknown>>({
    queryName: `Get${capitalizeFirstLetter(entityConfig?.service ?? '')}`,
    serviceOptions: {
      namespace: projectId,
      name: entityId,
    },
    clientOptions: {
      enabled: !!entityConfig?.service && !!entityId,
    },
  });

  // TODO: error handling for URLs that don't match any entity config
  const detailViewConfig =
    (entityConfig?.views ?? []).find((view) => view.type === 'detail') ?? undefined;

  if (error) {
    return (
      <ErrorView
        title="Entity not found"
        description={`Could not load ${entity} "${entityId}". ${error.message}`}
        illustration={
          <CircleExclamationMark
            kind={CircleExclamationMarkKind.ERROR}
            width={theme.sizing.scale1600}
          />
        }
        buttonConfig={{
          onClick: () => navigate(`/${projectId}/${phase}/${entity}`),
          content: 'Back to list',
        }}
      />
    );
  }

  const handleGoBack = () => {
    navigate(`/${projectId}/${phase}/${entity}`);
  };

  return (
    <DetailView
      subtitle={entityConfig!.name}
      title={entityId}
      onGoBack={handleGoBack}
      headerContent={
        <Row
          items={detailViewConfig!.metadata}
          record={data?.[entityConfig!.service] as Record<string, unknown>}
          loading={isLoading}
        />
      }
    >
      {detailViewConfig!.pages.map((page, index) => (
        <div key={index}>{page.type} will go here...</div>
      ))}
    </DetailView>
  );
}
