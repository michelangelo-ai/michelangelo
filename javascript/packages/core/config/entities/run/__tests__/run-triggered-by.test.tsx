import { render, screen, within } from '@testing-library/react';

import { TRIGGERED_BY_LABEL } from '#core/config/entities/run/shared';
import { TRAIN_PHASE } from '#core/config/phases/train';
import { EntityDetailRoute } from '#core/router/entity-detail-route';
import { PhaseListRoute } from '#core/router/phase-list-route';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';

describe('Pipeline run "Triggered by"', () => {
  function buildTriggeredRun() {
    return {
      metadata: {
        name: 'triggered-run',
        namespace: 'myproject',
        labels: { [TRIGGERED_BY_LABEL]: 'nightly-trigger' },
      },
      status: { state: 3 },
    };
  }

  function buildManualRun() {
    return {
      metadata: { name: 'manual-run', namespace: 'myproject' },
      status: { state: 3 },
    };
  }

  it('links a triggered run back to its trigger in the runs list', async () => {
    render(
      <PhaseListRoute phases={{ train: TRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/runs' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            ListPipelineRun: { pipelineRunList: { items: [buildTriggeredRun()] } },
          }),
        }),
      ])
    );

    expect(await screen.findByRole('columnheader', { name: 'Triggered by' })).toBeInTheDocument();

    const link = await screen.findByRole('link', { name: 'nightly-trigger' });
    expect(link).toHaveAttribute('href', '/myproject/train/triggers/nightly-trigger');
  });

  it('renders no trigger link for a manually started run', async () => {
    render(
      <PhaseListRoute phases={{ train: TRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/runs' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            ListPipelineRun: { pipelineRunList: { items: [buildManualRun()] } },
          }),
        }),
      ])
    );

    const row = await screen.findByRole('row', { name: /manual-run/ });
    // The only link in the row is the run name itself — no dead trigger link.
    expect(within(row).getAllByRole('link')).toHaveLength(1);
    expect(within(row).getByRole('link', { name: 'manual-run' })).toBeInTheDocument();
  });

  it('shows the trigger link in the run detail header', async () => {
    render(
      <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/runs/triggered-run' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetPipelineRun: { pipelineRun: buildTriggeredRun() },
          }),
        }),
      ])
    );

    const link = await screen.findByRole('link', { name: 'nightly-trigger' });
    expect(link).toHaveAttribute('href', '/myproject/train/triggers/nightly-trigger');
  });
});
