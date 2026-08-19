import { render, screen } from '@testing-library/react';
import { vi } from 'vitest';

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
  describe('header', () => {
    const buildDeployment = (overrides = {}) => ({
      metadata: {
        name: 'sentiment-deployment',
        creationTimestamp: { seconds: 1746000000 },
        labels: { 'michelangelo/owner': 'user-example' },
      },
      status: {
        state: 'DEPLOYMENT_STATE_HEALTHY',
        stage: 'DEPLOYMENT_STAGE_ROLLOUT_COMPLETE',
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
  });

  describe('information tab', () => {
    const buildDeployment = () => ({
      metadata: {
        name: 'sentiment-deployment',
        creationTimestamp: { seconds: 1746000000 },
        labels: { 'michelangelo/owner': 'user-example' },
      },
      spec: {
        definition: { type: 'TARGET_TYPE_INFERENCE_SERVER' },
        strategy: { rolloutStrategy: { case: 'rolling', value: {} } },
        target: { case: 'inferenceServer', value: { name: 'triton-server' } },
        desiredRevision: { name: 'sentiment-model-rev-3' },
        resourceLinks: { Dashboard: 'https://grafana.example.com/d/abc' },
      },
      status: {
        state: 'DEPLOYMENT_STATE_HEALTHY',
        stage: 'DEPLOYMENT_STAGE_ROLLOUT_COMPLETE',
        message: 'Rollout completed successfully.',
        currentRevision: { name: 'sentiment-model-rev-2' },
        conditions: [] as object[],
      },
    });

    const infoTabResponses = () => ({
      GetDeployment: { deployment: buildDeployment() },
    });

    it('renders the configuration details', async () => {
      render(
        <EntityDetailRoute phases={{ deploy: DEPLOY_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/deploy/deployments/sentiment-deployment/info',
          }),
          getServiceProviderWrapper({ request: createQueryMockRouter(infoTabResponses()) }),
        ])
      );

      expect(await screen.findByText('Configuration')).toBeInTheDocument();
      expect(await screen.findByLabelText('Type of deployment')).toHaveDisplayValue('Online');
    });

    it('renders the target link in useful links', async () => {
      render(
        <EntityDetailRoute phases={{ deploy: DEPLOY_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/deploy/deployments/sentiment-deployment/info',
          }),
          getServiceProviderWrapper({ request: createQueryMockRouter(infoTabResponses()) }),
        ])
      );

      expect(await screen.findByRole('link', { name: 'triton-server' })).toHaveAttribute(
        'href',
        '/myproject/deploy/targets/triton-server'
      );
    });

    it('shows a loading state until the deployment data resolves', async () => {
      render(
        <EntityDetailRoute phases={{ deploy: DEPLOY_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/deploy/deployments/sentiment-deployment/info',
          }),
          getServiceProviderWrapper({
            // Never resolves, so the page stays in its loading state.
            request: vi.fn().mockReturnValue(new Promise<never>(() => undefined)),
          }),
        ])
      );

      expect(await screen.findByText('Configuration')).toBeInTheDocument();
      expect(screen.queryByLabelText('Type of deployment')).not.toBeInTheDocument();
      expect(screen.queryByRole('link', { name: 'triton-server' })).not.toBeInTheDocument();
    });

    it('renders the revision references on the deployment', async () => {
      render(
        <EntityDetailRoute phases={{ deploy: DEPLOY_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/deploy/deployments/sentiment-deployment/info',
          }),
          getServiceProviderWrapper({ request: createQueryMockRouter(infoTabResponses()) }),
        ])
      );

      expect(await screen.findByText('sentiment-model-rev-2')).toBeInTheDocument();
      expect(screen.getByText('Current model in production')).toBeInTheDocument();
      expect(screen.getByText('sentiment-model-rev-3')).toBeInTheDocument();
      expect(screen.getByText('No model currently being deployed')).toBeInTheDocument();
    });

    it('renders empty states when no revisions are set', async () => {
      render(
        <EntityDetailRoute phases={{ deploy: DEPLOY_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/deploy/deployments/sentiment-deployment/info',
          }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetDeployment: {
                deployment: {
                  metadata: { name: 'sentiment-deployment' },
                  spec: { definition: { type: 'TARGET_TYPE_INFERENCE_SERVER' } },
                  status: { state: 'DEPLOYMENT_STATE_EMPTY', stage: 'DEPLOYMENT_STAGE_INVALID' },
                },
              },
            }),
          }),
        ])
      );

      expect(await screen.findByText('No currently deployed model')).toBeInTheDocument();
      expect(screen.getByText('No model currently being deployed')).toBeInTheDocument();
      expect(screen.getByText('No model configured to be deployed')).toBeInTheDocument();
    });
  });

  describe('ongoing operations tab', () => {
    const buildDeployment = (overrides = {}) => ({
      metadata: {
        name: 'sentiment-deployment',
        creationTimestamp: { seconds: 1746000000 },
        labels: { 'michelangelo/owner': 'user-example' },
      },
      status: {
        state: 'DEPLOYMENT_STATE_HEALTHY',
        stage: 'DEPLOYMENT_STAGE_ROLLOUT_COMPLETE',
        conditions: [] as object[],
      },
      ...overrides,
    });

    it('renders the stages for the deployment', async () => {
      render(
        <EntityDetailRoute phases={{ deploy: DEPLOY_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/deploy/deployments/sentiment-deployment/ongoing-operations',
          }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetDeployment: {
                deployment: buildDeployment({
                  status: {
                    state: 'DEPLOYMENT_STATE_HEALTHY',
                    stage: 'DEPLOYMENT_STAGE_ROLLOUT_COMPLETE',
                    conditions: [
                      {
                        type: 'Validation',
                        status: 'CONDITION_STATUS_TRUE',
                        lastUpdatedTimestamp: '1746000600000',
                      },
                      {
                        type: 'Placement',
                        status: 'CONDITION_STATUS_UNKNOWN',
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

      expect(await screen.findByRole('tab', { name: 'Ongoing operations' })).toBeInTheDocument();
      await screen.findAllByText('Validation');
      await screen.findAllByText('Placement');
    });

    it('renders the Information and Details fields within a deployment stage', async () => {
      render(
        <EntityDetailRoute phases={{ deploy: DEPLOY_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/deploy/deployments/sentiment-deployment/ongoing-operations',
          }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetDeployment: {
                deployment: buildDeployment({
                  status: {
                    state: 'DEPLOYMENT_STATE_HEALTHY',
                    stage: 'DEPLOYMENT_STAGE_ROLLOUT_COMPLETE',
                    conditions: [
                      {
                        type: 'Placement',
                        status: 'CONDITION_STATUS_UNKNOWN',
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

      expect(await screen.findByText('Placing on inference server.')).toBeInTheDocument();
      expect(screen.getAllByText('Information').length).toBeGreaterThan(1);
      expect(screen.getByText('Details')).toBeInTheDocument();
      expect(screen.getByText('PlacementInProgress')).toBeInTheDocument();
    });

    it('renders stages when rollout has failed', async () => {
      render(
        <EntityDetailRoute phases={{ deploy: DEPLOY_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/deploy/deployments/sentiment-deployment/ongoing-operations',
          }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetDeployment: {
                deployment: buildDeployment({
                  status: {
                    state: 'DEPLOYMENT_STATE_UNHEALTHY',
                    stage: 'DEPLOYMENT_STAGE_ROLLOUT_FAILED',
                    conditions: [],
                    conditionsSnapshot: [
                      {
                        type: 'SnapshotValidation',
                        status: 'CONDITION_STATUS_TRUE',
                        lastUpdatedTimestamp: '1746000600000',
                      },
                      {
                        type: 'SnapshotPlacement',
                        status: 'CONDITION_STATUS_FALSE',
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
});
