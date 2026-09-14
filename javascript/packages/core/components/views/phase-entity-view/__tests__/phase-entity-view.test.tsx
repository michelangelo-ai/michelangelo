import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { CellType } from '#core/components/cell/constants';
import { interpolate } from '#core/interpolation/interpolate';
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

  describe('list variants', () => {
    const buildVariantEntity = () =>
      buildPipelineEntityConfig({
        views: [
          {
            type: 'list',
            tableConfig: { columns: [{ id: 'metadata.name', label: 'Name' }] },
            variants: [
              { id: 'pipelines', label: 'Pipelines' },
              {
                id: 'revisions',
                label: 'Revisions',
                service: 'revision',
                tableConfig: {
                  columns: [{ id: 'spec.baseResource.name', label: 'Pipeline' }],
                },
                serviceOptions: { listOptions: { limit: '250' } },
              },
            ],
          },
        ],
      }) as ListableEntity;

    const renderView = (location: string) => {
      const request = createQueryMockRouter({
        ListPipeline: { pipelineList: { items: [{ metadata: { name: 'my-pipeline' } }] } },
        ListRevision: {
          revisionList: { items: [{ spec: { baseResource: { name: 'my-pipeline' } } }] },
        },
      });
      render(
        <PhaseEntityView
          phaseConfig={buildPhaseConfig({ pipelineTypes: ['PIPELINE_TYPE_TRAIN'] })}
          entities={[buildVariantEntity()]}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getErrorProviderWrapper(),
          getServiceProviderWrapper({ request }),
          getRouterWrapper({ location }),
          getIconProviderWrapper(),
        ])
      );
      return request;
    };

    it('renders the first variant by default with a segmented control', async () => {
      const request = renderView('/project-1/training/pipelines');

      expect(await screen.findByRole('cell', { name: 'my-pipeline' })).toBeInTheDocument();
      expect(screen.getByRole('columnheader', { name: /Name/ })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'Pipelines' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'Revisions' })).toBeInTheDocument();
      expect(request).toHaveBeenCalledWith(
        'ListPipeline',
        expect.objectContaining({
          listOptions: { fieldSelector: 'pipeline_type in (PIPELINE_TYPE_TRAIN)' },
        }),
        expect.anything()
      );
    });

    it('switches data source and scopes the query when a segment is clicked', async () => {
      const user = userEvent.setup();
      const request = renderView('/project-1/training/pipelines');
      await screen.findByRole('cell', { name: 'my-pipeline' });

      await user.click(screen.getByRole('option', { name: 'Revisions' }));

      expect(await screen.findByRole('columnheader', { name: /Pipeline/ })).toBeInTheDocument();
      expect(request).toHaveBeenCalledWith(
        'ListRevision',
        expect.objectContaining({
          listOptions: {
            limit: '250',
            fieldSelector: 'base_type=Pipeline',
            labelSelector: 'michelangelo/PipelineType in (PIPELINE_TYPE_TRAIN)',
          },
        }),
        expect.anything()
      );
    });
  });

  it('opens the action component when an action menu item is clicked', async () => {
    const user = userEvent.setup();
    const RunDialog = () => <div role="dialog">Run dialog</div>;

    render(
      <PhaseEntityView
        phaseConfig={buildPhaseConfig()}
        entities={[
          buildPipelineEntityConfig({
            actions: [
              { display: { label: 'Run' }, modal: { type: 'custom', component: RunDialog } },
            ],
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

  it('resolves interpolated disabled conditions per-row', async () => {
    const user = userEvent.setup();
    const StubDialog = () => <div role="dialog">Stub</div>;

    render(
      <PhaseEntityView
        phaseConfig={buildPhaseConfig()}
        entities={[
          buildPipelineEntityConfig({
            actions: [
              {
                display: { label: 'Delete' },
                modal: { type: 'custom', component: StubDialog },
                disabled: [
                  {
                    condition: interpolate(
                      ({ data }) => (data as { locked?: boolean } | undefined)?.locked === true
                    ),
                    message: 'Record is locked',
                  },
                ],
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
            ListPipeline: { pipelineList: { items: [{ locked: true }] } },
          }),
        }),
        getRouterWrapper({ location: '/project-1/training/pipeline' }),
        getIconProviderWrapper(),
      ])
    );

    await user.click(await screen.findByRole('button', { name: 'Actions' }));
    const option = await screen.findByRole('option', { name: 'Delete' });
    await user.hover(option);
    expect(await screen.findByText('Record is locked')).toBeInTheDocument();
  });
});
