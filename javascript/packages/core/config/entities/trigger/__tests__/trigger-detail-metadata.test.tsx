import { render, screen } from '@testing-library/react';

import { RETRAIN_PHASE } from '#core/config/phases/retrain';
import { EntityDetailRoute } from '#core/router/entity-detail-route';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';

describe('Trigger detail page metadata header', () => {
  const buildTriggerRun = (overrides: Record<string, unknown> = {}) => ({
    metadata: { name: 'nightly-trigger', namespace: 'myproject' },
    spec: { pipeline: { name: 'my-pipeline', namespace: 'myproject' } },
    status: { state: 1 },
    ...overrides,
  });

  it('shows Cron for a cron-scheduled trigger and hides Interval seconds', async () => {
    const triggerRun = buildTriggerRun({
      spec: {
        pipeline: { name: 'my-pipeline', namespace: 'myproject' },
        trigger: { triggerType: { case: 'cronSchedule', value: { cron: '0 0 * * *' } } },
      },
    });

    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({ GetTriggerRun: { triggerRun } }),
        }),
      ])
    );

    expect(await screen.findByText('Cron')).toBeInTheDocument();
    expect(screen.getByText('0 0 * * *')).toBeInTheDocument();
    expect(screen.queryByText('Interval seconds')).not.toBeInTheDocument();
  });

  it('shows Interval seconds for an interval-scheduled trigger and hides Cron', async () => {
    const triggerRun = buildTriggerRun({
      spec: {
        pipeline: { name: 'my-pipeline', namespace: 'myproject' },
        trigger: {
          triggerType: { case: 'intervalSchedule', value: { interval: { seconds: 3600 } } },
        },
      },
    });

    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({ GetTriggerRun: { triggerRun } }),
        }),
      ])
    );

    expect(await screen.findByText('Interval seconds')).toBeInTheDocument();
    expect(screen.getByText('3600')).toBeInTheDocument();
    expect(screen.queryByText('Cron')).not.toBeInTheDocument();
  });

  it('shows Max concurrency and hides Batch size/Wait minutes when maxConcurrency is set', async () => {
    const triggerRun = buildTriggerRun({
      spec: {
        pipeline: { name: 'my-pipeline', namespace: 'myproject' },
        trigger: { maxConcurrency: 5, batchPolicy: { batchSize: 10, waitSeconds: 120 } },
      },
    });

    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({ GetTriggerRun: { triggerRun } }),
        }),
      ])
    );

    expect(await screen.findByText('Max concurrency')).toBeInTheDocument();
    expect(screen.getByText('5')).toBeInTheDocument();
    expect(screen.queryByText('Batch size')).not.toBeInTheDocument();
    expect(screen.queryByText('Wait minutes')).not.toBeInTheDocument();
  });

  it('shows Batch size/Wait minutes and hides Max concurrency when maxConcurrency is unset', async () => {
    const triggerRun = buildTriggerRun({
      spec: {
        pipeline: { name: 'my-pipeline', namespace: 'myproject' },
        trigger: { batchPolicy: { batchSize: 10, waitSeconds: 120 } },
      },
    });

    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({ GetTriggerRun: { triggerRun } }),
        }),
      ])
    );

    expect(await screen.findByText('Batch size')).toBeInTheDocument();
    expect(screen.getByText('10')).toBeInTheDocument();
    expect(screen.getByText('Wait minutes')).toBeInTheDocument();
    // 120 seconds -> 2 minutes.
    expect(screen.getByText('2')).toBeInTheDocument();
    expect(screen.queryByText('Max concurrency')).not.toBeInTheDocument();
  });

  it('shows Start time and End time for a backfill-created trigger run', async () => {
    const triggerRun = buildTriggerRun({
      spec: {
        pipeline: { name: 'my-pipeline', namespace: 'myproject' },
        startTimestamp: { seconds: '1700000000' },
        endTimestamp: { seconds: '1700003600' },
      },
    });

    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({ GetTriggerRun: { triggerRun } }),
        }),
      ])
    );

    expect(await screen.findByText('Start time')).toBeInTheDocument();
    expect(screen.getByText('End time')).toBeInTheDocument();
  });

  it('hides Start time and End time for a normal scheduled trigger run', async () => {
    const triggerRun = buildTriggerRun();

    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({ GetTriggerRun: { triggerRun } }),
        }),
      ])
    );

    await screen.findByText('State');
    expect(screen.queryByText('Start time')).not.toBeInTheDocument();
    expect(screen.queryByText('End time')).not.toBeInTheDocument();
  });

  it('renders the Information tab before the Triggered Runs tab', async () => {
    const triggerRun = buildTriggerRun();

    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({ GetTriggerRun: { triggerRun } }),
        }),
      ])
    );

    const tabs = await screen.findAllByRole('tab');
    expect(tabs.map((tab) => tab.textContent)).toEqual(['Information', 'Triggered Runs']);
  });
});
