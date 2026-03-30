import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { CellType } from '#core/components/cell/constants';
import {
  buildEntityConfigFactory,
  buildPhaseConfigFactory,
} from '#core/router/__fixtures__/phase-config-factory';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';
import { PhaseEntityView } from '../phase-entity-view';

import type { ActionComponentProps } from '#core/components/actions/types';
import type { ListableEntity } from '../types';

describe('PhaseEntityView', () => {
  const buildPhaseConfig = buildPhaseConfigFactory({ id: 'training', name: 'Train & Evaluate' });
  const buildPipelineEntityConfig = buildEntityConfigFactory({
    id: 'pipelines',
    name: 'Pipelines',
    service: 'pipeline',
  });
  const buildModelEntityConfig = buildEntityConfigFactory({
    id: 'models',
    name: 'Models',
    service: 'model',
  });

  it('renders a tab for each entity', () => {
    render(
      <PhaseEntityView
        phaseConfig={buildPhaseConfig()}
        entities={[buildPipelineEntityConfig(), buildModelEntityConfig()] as ListableEntity[]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getServiceProviderWrapper({
          request: createQueryMockRouter({ ListPipeline: { pipelineList: { items: [] } } }),
        }),
        getRouterWrapper({ location: '/project-1/training/pipeline' }),
        getIconProviderWrapper(),
      ])
    );

    expect(screen.getByRole('tab', { name: 'Pipelines' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Models' })).toBeInTheDocument();
  });

  it('renders data fetched from the API in the table', async () => {
    render(
      <PhaseEntityView
        phaseConfig={buildPhaseConfig()}
        entities={[
          buildPipelineEntityConfig({
            views: [
              {
                type: 'list',
                tableConfig: { columns: [{ id: 'name', label: 'Name', type: CellType.TEXT }] },
              },
            ],
          }) as ListableEntity,
        ]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            ListPipeline: { pipelineList: { items: [{ name: 'my-pipeline' }] } },
          }),
        }),
        getRouterWrapper({ location: '/project-1/training/pipeline' }),
        getIconProviderWrapper(),
      ])
    );

    expect(await screen.findByRole('cell', { name: 'my-pipeline' })).toBeInTheDocument();
  });

  it('opens the action component when an action menu item is clicked', async () => {
    const user = userEvent.setup();
    const RunDialog = ({ isOpen }: ActionComponentProps) =>
      isOpen ? <div role="dialog">Run dialog</div> : null;

    render(
      <PhaseEntityView
        phaseConfig={buildPhaseConfig()}
        entities={[
          buildPipelineEntityConfig({
            actions: [{ display: { label: 'Run' }, component: RunDialog }],
          }) as ListableEntity,
        ]}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            ListPipeline: { pipelineList: { items: [{}] } },
          }),
        }),
        getRouterWrapper({ location: '/project-1/training/pipeline' }),
        getIconProviderWrapper(),
      ])
    );

    await user.click(await screen.findByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Run' }));
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
  });
});
