import { render, screen } from '@testing-library/react';
import { vi } from 'vitest';

import { CONDITION_STATUS, INFERENCE_SERVER_STATE } from '#core/config/entities/targets/shared';
import { DEPLOY_PHASE } from '#core/config/phases/deploy';
import { EntityDetailRoute } from '#core/router/entity-detail-route';
import { PhaseListRoute } from '#core/router/phase-list-route';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';

describe('Target list page', () => {
  it('renders the Targets tab', () => {
    render(
      <PhaseListRoute phases={{ deploy: DEPLOY_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/deploy/targets' }),
        getServiceProviderWrapper({
          request: vi.fn().mockResolvedValue({ inferenceServerList: { items: [] } }),
        }),
      ])
    );

    expect(screen.getByRole('tab', { name: 'Targets' })).toBeInTheDocument();
  });

  it('renders the correct column headers', async () => {
    render(
      <PhaseListRoute phases={{ deploy: DEPLOY_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/deploy/targets' }),
        getServiceProviderWrapper({
          request: vi.fn().mockResolvedValue({ inferenceServerList: { items: [] } }),
        }),
      ])
    );

    expect(await screen.findByRole('columnheader', { name: 'Target name' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Last updated' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Type' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'State' })).toBeInTheDocument();
  });

  it('renders a link to the target detail page on the target name', async () => {
    render(
      <PhaseListRoute phases={{ deploy: DEPLOY_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/deploy/targets' }),
        getServiceProviderWrapper({
          request: vi.fn().mockResolvedValue({
            inferenceServerList: {
              items: [{ metadata: { name: 'sentiment-target' } }],
            },
          }),
        }),
      ])
    );

    const link = await screen.findByRole('link', { name: 'sentiment-target' });
    expect(link).toHaveAttribute('href', '/myproject/deploy/targets/sentiment-target');
  });
});

describe('Target detail page', () => {
  const buildTarget = (overrides = {}) => ({
    metadata: {
      name: 'sentiment-target',
    },
    spec: {
      owner: { name: 'user-example' },
    },
    status: {
      state: INFERENCE_SERVER_STATE.SERVING,
      createTime: '2025-04-30T12:00:00Z',
      conditions: [] as object[],
    },
    ...overrides,
  });

  it('renders details for target', async () => {
    render(
      <EntityDetailRoute phases={{ deploy: DEPLOY_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({
          location: '/myproject/deploy/targets/sentiment-target/stages',
        }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetInferenceServer: { inferenceServer: buildTarget() },
          }),
        }),
      ])
    );

    expect(screen.getByText('sentiment-target')).toBeInTheDocument();
    expect(await screen.findByText('Created')).toBeInTheDocument();
    expect(screen.getByText('Owner')).toBeInTheDocument();
    expect(screen.getByText('State')).toBeInTheDocument();
  });

  it('renders the stages for the target', async () => {
    render(
      <EntityDetailRoute phases={{ deploy: DEPLOY_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({
          location: '/myproject/deploy/targets/sentiment-target/stages',
        }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetInferenceServer: {
              inferenceServer: buildTarget({
                status: {
                  state: INFERENCE_SERVER_STATE.SERVING,
                  createTime: '2025-04-30T12:00:00Z',
                  conditions: [
                    {
                      type: 'ModelLoaded',
                      status: CONDITION_STATUS.TRUE,
                      lastUpdatedTimestamp: '1746000600000',
                    },
                    {
                      type: 'ServerReady',
                      status: CONDITION_STATUS.UNKNOWN,
                      message: 'Waiting for server to become ready.',
                      reason: 'ServerNotReady',
                      lastUpdatedTimestamp: '1746002400000',
                    },
                  ],
                },
              }),
            },
          }),
        }),
      ])
    );

    expect(await screen.findByRole('tab', { name: 'Stages' })).toBeInTheDocument();
    await screen.findAllByText('ModelLoaded');
    await screen.findAllByText('ServerReady');
  });

  it('renders the Information and Details fields within a target stage', async () => {
    render(
      <EntityDetailRoute phases={{ deploy: DEPLOY_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({
          location: '/myproject/deploy/targets/sentiment-target/stages',
        }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetInferenceServer: {
              inferenceServer: buildTarget({
                status: {
                  state: INFERENCE_SERVER_STATE.SERVING,
                  createTime: '2025-04-30T12:00:00Z',
                  conditions: [
                    {
                      type: 'ServerReady',
                      status: CONDITION_STATUS.UNKNOWN,
                      message: 'Waiting for server to become ready.',
                      reason: 'ServerNotReady',
                      lastUpdatedTimestamp: '1746002400000',
                    },
                  ],
                },
              }),
            },
          }),
        }),
      ])
    );

    expect(await screen.findByText('Information')).toBeInTheDocument();
    expect(screen.getByText('Waiting for server to become ready.')).toBeInTheDocument();
    expect(screen.getByText('Details')).toBeInTheDocument();
    expect(screen.getByText('ServerNotReady')).toBeInTheDocument();
  });

  it('renders the empty state when no stages are reported', async () => {
    render(
      <EntityDetailRoute phases={{ deploy: DEPLOY_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({
          location: '/myproject/deploy/targets/sentiment-target/stages',
        }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetInferenceServer: { inferenceServer: buildTarget() },
          }),
        }),
      ])
    );

    expect(await screen.findByText('No stages reported')).toBeInTheDocument();
    expect(
      screen.getByText('Stages will appear here once the inference server is initialized')
    ).toBeInTheDocument();
  });
});
