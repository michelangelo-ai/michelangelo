import { render, screen } from '@testing-library/react';
import { vi } from 'vitest';

import {
  DEPLOYMENT_CONDITION_STATUS,
  DEPLOYMENT_STAGE,
  DEPLOYMENT_STATE,
} from '#core/config/entities/deployment/shared';
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

describe('Deployment list page', () => {
  it('renders the Deployments tab', () => {
    render(
      <PhaseListRoute phases={{ deploy: DEPLOY_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/deploy/deployments' }),
        getServiceProviderWrapper({
          request: vi.fn().mockResolvedValue({ deploymentList: { items: [] } }),
        }),
      ])
    );

    expect(screen.getByRole('tab', { name: 'Deployments' })).toBeInTheDocument();
  });

  it('renders the correct column headers', async () => {
    render(
      <PhaseListRoute phases={{ deploy: DEPLOY_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/deploy/deployments' }),
        getServiceProviderWrapper({
          request: vi.fn().mockResolvedValue({ deploymentList: { items: [] } }),
        }),
      ])
    );

    expect(await screen.findByRole('columnheader', { name: 'Name' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Model' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Type' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Stage' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Target' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Owner' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'State' })).toBeInTheDocument();
  });

  it('renders a link to the deployment detail page on the deployment name', async () => {
    render(
      <PhaseListRoute phases={{ deploy: DEPLOY_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/deploy/deployments' }),
        getServiceProviderWrapper({
          request: vi.fn().mockResolvedValue({
            deploymentList: {
              items: [{ metadata: { name: 'sentiment-deployment' } }],
            },
          }),
        }),
      ])
    );

    const link = await screen.findByRole('link', { name: 'sentiment-deployment' });
    expect(link).toHaveAttribute('href', '/myproject/deploy/deployments/sentiment-deployment');
  });
});

describe('Deployment detail page', () => {
  const buildDeployment = (overrides = {}) => ({
    metadata: {
      name: 'sentiment-deployment',
      creationTimestamp: { seconds: 1746000000 },
      labels: { 'michelangelo/owner': 'user-example' },
    },
    status: {
      state: DEPLOYMENT_STATE.HEALTHY,
      stage: DEPLOYMENT_STAGE.ROLLOUT_COMPLETE,
      conditions: [] as object[],
    },
    ...overrides,
  });

  it('renders details for deployment', async () => {
    render(
      <EntityDetailRoute phases={{ deploy: DEPLOY_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({
          location: '/myproject/deploy/deployments/sentiment-deployment/stages',
        }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetDeployment: { deployment: buildDeployment() },
          }),
        }),
      ])
    );

    expect(screen.getByText('sentiment-deployment')).toBeInTheDocument();
    expect(await screen.findByText('Created')).toBeInTheDocument();
    expect(screen.getByText('Owner')).toBeInTheDocument();
    expect(screen.getByText('Stage')).toBeInTheDocument();
    expect(screen.getByText('State')).toBeInTheDocument();
  });

  it('renders the stages for the deployment', async () => {
    render(
      <EntityDetailRoute phases={{ deploy: DEPLOY_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({
          location: '/myproject/deploy/deployments/sentiment-deployment/stages',
        }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetDeployment: {
              deployment: buildDeployment({
                status: {
                  state: DEPLOYMENT_STATE.HEALTHY,
                  stage: DEPLOYMENT_STAGE.ROLLOUT_COMPLETE,
                  conditions: [
                    {
                      type: 'Validation',
                      status: DEPLOYMENT_CONDITION_STATUS.TRUE,
                      lastUpdatedTimestamp: '1746000600000',
                    },
                    {
                      type: 'Placement',
                      status: DEPLOYMENT_CONDITION_STATUS.UNKNOWN,
                      message: 'Placing on inference server.',
                      reason: 'PlacementInProgress',
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
    await screen.findAllByText('Validation');
    await screen.findAllByText('Placement');
  });

  it('renders the Information and Details fields within a deployment stage', async () => {
    render(
      <EntityDetailRoute phases={{ deploy: DEPLOY_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({
          location: '/myproject/deploy/deployments/sentiment-deployment/stages',
        }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetDeployment: {
              deployment: buildDeployment({
                status: {
                  state: DEPLOYMENT_STATE.HEALTHY,
                  stage: DEPLOYMENT_STAGE.ROLLOUT_COMPLETE,
                  conditions: [
                    {
                      type: 'Placement',
                      status: DEPLOYMENT_CONDITION_STATUS.UNKNOWN,
                      message: 'Placing on inference server.',
                      reason: 'PlacementInProgress',
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
    expect(screen.getByText('Placing on inference server.')).toBeInTheDocument();
    expect(screen.getByText('Details')).toBeInTheDocument();
    expect(screen.getByText('PlacementInProgress')).toBeInTheDocument();
  });

  it('renders stages when rollout has failed', async () => {
    render(
      <EntityDetailRoute phases={{ deploy: DEPLOY_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({
          location: '/myproject/deploy/deployments/sentiment-deployment/stages',
        }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetDeployment: {
              deployment: buildDeployment({
                status: {
                  state: DEPLOYMENT_STATE.UNHEALTHY,
                  stage: DEPLOYMENT_STAGE.ROLLOUT_FAILED,
                  conditions: [],
                  conditionsSnapshot: [
                    {
                      type: 'SnapshotValidation',
                      status: DEPLOYMENT_CONDITION_STATUS.TRUE,
                      lastUpdatedTimestamp: '1746000600000',
                    },
                    {
                      type: 'SnapshotPlacement',
                      status: DEPLOYMENT_CONDITION_STATUS.FALSE,
                      message: 'Failed to place on inference server.',
                      reason: 'NoCapacity',
                      lastUpdatedTimestamp: '1746001200000',
                    },
                  ],
                },
              }),
            },
          }),
        }),
      ])
    );

    await screen.findAllByText('SnapshotValidation');
    await screen.findAllByText('SnapshotPlacement');
    await screen.findByText('NoCapacity');
  });
});
