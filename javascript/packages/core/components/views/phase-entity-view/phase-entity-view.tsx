import { useEffect } from 'react';
import { useNavigate } from 'react-router-dom-v5-compat';
import { useStyletron } from 'baseui';
import { Tab, Tabs } from 'baseui/tabs-motion';

import { ErrorView } from '#core/components/error-view/error-view';
import { CircleExclamationMark } from '#core/components/illustrations/circle-exclamation-mark/circle-exclamation-mark';
import { CircleExclamationMarkKind } from '#core/components/illustrations/circle-exclamation-mark/types';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { EntityTable } from './entity-table';

import type { PhaseEntityViewProps } from './types';

/**
 * Renders tabbed interface for phase entities with URL-synchronized navigation.
 *
 * Expects to receive only active entities with list views. Auto-redirects to first
 * entity if no entity in URL to prevent empty states.
 */
export function PhaseEntityView<T extends object = object>({
  phaseId,
  entities,
}: PhaseEntityViewProps<T>) {
  const [, theme] = useStyletron();
  const navigate = useNavigate();
  const { projectId, entity: currentEntity } = useStudioParams('list');

  useEffect(() => {
    if (!currentEntity) {
      navigate(`/${projectId}/${phaseId}/${entities[0].id}`);
    }
  }, [currentEntity, navigate, projectId, phaseId, entities]);

  const currentEntityIndex = entities.findIndex((entity) => entity.id === currentEntity);
  const activeKey = currentEntityIndex >= 0 ? currentEntityIndex.toString() : '0';

  const routeToEntity = ({ activeKey }: { activeKey: React.Key }) => {
    const selectedEntity = entities[Number(activeKey)];
    if (selectedEntity) {
      navigate(`/${projectId}/${phaseId}/${selectedEntity.id}`);
    }
  };

  const currentEntityConfig = entities.find((entity) => entity.id === currentEntity);
  if (!currentEntityConfig) {
    return (
      <ErrorView
        buttonConfig={{
          onClick: () => navigate(`/${projectId}`),
          content: 'Go home',
        }}
        description={`Entity "${currentEntity}" not found`}
        illustration={
          <CircleExclamationMark
            kind={CircleExclamationMarkKind.ERROR}
            width={theme.sizing.scale1600}
          />
        }
        title="Entity not found"
      />
    );
  }

  return (
    <Tabs activeKey={activeKey} onChange={routeToEntity}>
      {entities.map((entity, index) => (
        <Tab key={String(index)} title={entity.name}>
          {String(index) === activeKey && (
            <EntityTable<T>
              service={entity.service}
              tableConfig={currentEntityConfig.views[0].tableConfig}
              tableSettingsId={`${phaseId}/${entity.id}`}
            />
          )}
        </Tab>
      ))}
    </Tabs>
  );
}
