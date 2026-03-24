import { useNavigate } from 'react-router-dom-v5-compat';
import { useStyletron } from 'baseui';

import { ErrorView } from '#core/components/error-view/error-view';
import { CircleExclamationMark } from '#core/components/illustrations/circle-exclamation-mark/circle-exclamation-mark';
import { CircleExclamationMarkKind } from '#core/components/illustrations/circle-exclamation-mark/types';
import { PhaseEntityView } from '#core/components/views/phase-entity-view/phase-entity-view';
import { isListableEntity } from '#core/components/views/phase-entity-view/utils';
import { PHASES } from '#core/config/phases/phases';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';

import type { PhaseConfig } from '#core/types/common/studio-types';

/**
 * Route component that maps phase URL parameters to entity configurations.
 *
 * Handles unknown phases and phases with no active list entities by showing
 * appropriate messages rather than rendering empty UI. Only renders PhaseEntityView
 * when there are valid entities to display.
 *
 * @param phaseEntityConfig - Optional phase entity configuration override for testing
 */
export function PhaseListRoute({ phases = PHASES }: { phases?: Record<string, PhaseConfig> } = {}) {
  const [, theme] = useStyletron();
  const { phase, projectId } = useStudioParams('list');
  const navigate = useNavigate();

  if (!(phase in phases)) {
    return (
      <ErrorView
        title="Phase not found"
        description={`Phase "${phase}" configuration not found. Available phases: ${Object.keys(phases).join(', ')}`}
        illustration={
          <CircleExclamationMark
            kind={CircleExclamationMarkKind.ERROR}
            width={theme.sizing.scale1600}
            height={theme.sizing.scale1600}
          />
        }
        buttonConfig={{
          onClick: () => navigate(`/${projectId}`),
          content: 'Go home',
        }}
      />
    );
  }

  const listableEntities = phases[phase].entities.filter(isListableEntity);

  if (listableEntities.length === 0) {
    return (
      <ErrorView
        title="Phase has no active entities"
        description={`Phase "${phase}" has no active entities with list views configured.`}
        illustration={
          <CircleExclamationMark
            kind={CircleExclamationMarkKind.ERROR}
            width={theme.sizing.scale1600}
            height={theme.sizing.scale1600}
          />
        }
        buttonConfig={{
          onClick: () => navigate(`/${projectId}`),
          content: 'Go home',
        }}
      />
    );
  }

  return <PhaseEntityView phaseConfig={phases[phase]} entities={listableEntities} />;
}
