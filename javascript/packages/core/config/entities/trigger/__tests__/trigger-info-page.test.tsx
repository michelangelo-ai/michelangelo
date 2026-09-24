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

describe('Trigger detail "Information" tab', () => {
  const buildTriggerRun = (overrides: Record<string, unknown> = {}) => ({
    metadata: { name: 'nightly-trigger', namespace: 'myproject' },
    spec: { pipeline: { name: 'my-pipeline', namespace: 'myproject' } },
    status: { state: 1 },
    ...overrides,
  });

  it('renders a log link when status.logUrl is set', async () => {
    const triggerRun = buildTriggerRun({
      status: { state: 1, logUrl: 'https://workflow.example.com/trigger-1' },
    });

    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({
          location: '/myproject/retrain/triggers/nightly-trigger/information',
        }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({ GetTriggerRun: { triggerRun } }),
        }),
      ])
    );

    const logLink = await screen.findByRole('link', { name: 'Trigger run logs' });
    expect(logLink).toHaveAttribute('href', 'https://workflow.example.com/trigger-1');
  });

  it('omits the log link entirely when status.logUrl is unset', async () => {
    const triggerRun = buildTriggerRun();

    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({
          location: '/myproject/retrain/triggers/nightly-trigger/information',
        }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({ GetTriggerRun: { triggerRun } }),
        }),
      ])
    );

    // Wait for the page to finish loading before asserting on an absence.
    await screen.findByText('Useful links');
    expect(screen.queryByRole('link', { name: 'Trigger run logs' })).not.toBeInTheDocument();
  });

  it('shows the error message section only when status.errorMessage is present', async () => {
    const triggerRun = buildTriggerRun({
      status: { state: 3, errorMessage: 'Task train failed:\nOOMKilled' },
    });

    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({
          location: '/myproject/retrain/triggers/nightly-trigger/information',
        }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({ GetTriggerRun: { triggerRun } }),
        }),
      ])
    );

    expect(await screen.findByText('Message')).toBeInTheDocument();
    expect(screen.getByLabelText('Error message')).toHaveValue('Task train failed:\nOOMKilled');
  });

  it('hides the error message section when status.errorMessage is absent', async () => {
    const triggerRun = buildTriggerRun();

    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({
          location: '/myproject/retrain/triggers/nightly-trigger/information',
        }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({ GetTriggerRun: { triggerRun } }),
        }),
      ])
    );

    await screen.findByText('Useful links');
    expect(screen.queryByText('Message')).not.toBeInTheDocument();
  });

  it('renders spec.trigger.parametersMap as formatted JSON', async () => {
    const triggerRun = buildTriggerRun({
      spec: {
        pipeline: { name: 'my-pipeline', namespace: 'myproject' },
        trigger: { parametersMap: { 'daily-01': { learningRate: 0.01 } } },
      },
    });

    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({
          location: '/myproject/retrain/triggers/nightly-trigger/information',
        }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({ GetTriggerRun: { triggerRun } }),
        }),
      ])
    );

    expect(await screen.findByText('Parameters')).toBeInTheDocument();
    expect(document.body).toHaveTextContent('"daily-01"');
    expect(document.body).toHaveTextContent('"learningRate": 0.01');
  });

  it('omits the Parameters section when parametersMap is empty or absent', async () => {
    const triggerRun = buildTriggerRun();

    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({
          location: '/myproject/retrain/triggers/nightly-trigger/information',
        }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({ GetTriggerRun: { triggerRun } }),
        }),
      ])
    );

    await screen.findByText('Useful links');
    expect(screen.queryByText('Parameters')).not.toBeInTheDocument();
  });
});
