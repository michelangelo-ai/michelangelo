import { render, screen, waitFor } from '@testing-library/react';
import { vi } from 'vitest';

import { TASK_STATE } from '#core/components/views/execution/constants';
import { DEPLOYMENT_DETAIL_CONFIG } from '#core/config/entities/deployment/detail';
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

import type { ExecutionDetailPageConfig } from '#core/components/views/detail-view/types/detail-view-schema-types';

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
  });

  describe('information tab', () => {
    const buildDeployment = () => ({
      metadata: {
        name: 'sentiment-deployment',
        creationTimestamp: { seconds: 1746000000 },
        labels: { 'michelangelo/owner': 'user-example' },
      },
      spec: {
        definition: { type: 1 },
        strategy: { rolloutStrategy: { case: 'rolling', value: {} } },
        target: { case: 'inferenceServer', value: { name: 'triton-server' } },
        desiredRevision: { name: 'sentiment-model-rev-3' },
        resourceLinks: { Dashboard: 'https://grafana.example.com/d/abc' },
      },
      status: {
        state: DEPLOYMENT_STATE.HEALTHY,
        stage: DEPLOYMENT_STAGE.ROLLOUT_COMPLETE,
        message: 'Rollout completed successfully.',
        currentRevision: { name: 'sentiment-model-rev-2' },
        conditions: [] as object[],
      },
    });

    const buildModel = () => ({
      metadata: { creationTimestamp: { seconds: 1746000000 } },
      spec: {
        owner: { name: 'model-owner' },
        kind: 2,
        sourcePipelineRun: { name: 'run-20260825-080000' },
      },
    });

    const infoTabResponses = () => ({
      GetDeployment: { deployment: buildDeployment() },
      GetModel: { model: buildModel() },
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

    it('renders the resolved model metadata on the revision cards', async () => {
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

      await waitFor(() => expect(screen.getAllByText('model-owner')).toHaveLength(2));
      expect(screen.getAllByText('Regression')).toHaveLength(2);
      expect(screen.getAllByText('run-20260825-080000')).toHaveLength(2);
      expect(screen.getAllByText('Creation time')).toHaveLength(2);
      expect(screen.getAllByText('Source pipeline run')).toHaveLength(2);
    });

    it('falls back to the bare revision name when the model cannot be resolved', async () => {
      render(
        <EntityDetailRoute phases={{ deploy: DEPLOY_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/deploy/deployments/sentiment-deployment/info',
          }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetDeployment: { deployment: buildDeployment() },
              GetModel: {},
            }),
          }),
        ])
      );

      expect(await screen.findByText('sentiment-model-rev-2')).toBeInTheDocument();
      expect(screen.getByText('sentiment-model-rev-3')).toBeInTheDocument();
      expect(screen.queryByText('Regression')).not.toBeInTheDocument();
      expect(screen.getAllByText('Creation time')).toHaveLength(2);
      expect(screen.getAllByText('Owner')).toHaveLength(3); // 2 cards + detail page header
      expect(screen.getAllByText('Type')).toHaveLength(2);
      expect(screen.getAllByText('Source pipeline run')).toHaveLength(2);
      // 4 unresolved fields per card × 2 cards, plus the detail page header's empty Owner
      expect(screen.getAllByText('—')).toHaveLength(9);
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
                  spec: { definition: { type: 1 } },
                  status: { state: DEPLOYMENT_STATE.EMPTY, stage: DEPLOYMENT_STAGE.INVALID },
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
        state: DEPLOYMENT_STATE.HEALTHY,
        stage: DEPLOYMENT_STAGE.ROLLOUT_COMPLETE,
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

    it('renders a state chip per condition mapped from the condition status', async () => {
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
                    state: DEPLOYMENT_STATE.INITIALIZING,
                    stage: DEPLOYMENT_STAGE.PLACEMENT,
                    conditions: [
                      { type: 'Validated', status: DEPLOYMENT_CONDITION_STATUS.TRUE },
                      { type: 'Placement', status: DEPLOYMENT_CONDITION_STATUS.UNKNOWN },
                      { type: 'RolloutCompleted', status: DEPLOYMENT_CONDITION_STATUS.FALSE },
                    ],
                  },
                }),
              },
            }),
          }),
        ])
      );

      expect(await screen.findByText('Succeeded')).toBeInTheDocument();
      expect(screen.getByText('Pending')).toBeInTheDocument();
      expect(screen.getByText('Running')).toBeInTheDocument();
    });

    it('falls back to live conditions when a failed rollout has an empty snapshot', async () => {
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
                    state: DEPLOYMENT_STATE.UNHEALTHY,
                    stage: DEPLOYMENT_STAGE.ROLLOUT_FAILED,
                    conditions: [
                      { type: 'LiveCondition', status: DEPLOYMENT_CONDITION_STATUS.FALSE },
                    ],
                    conditionsSnapshot: [],
                  },
                }),
              },
            }),
          }),
        ])
      );

      await screen.findAllByText('LiveCondition');
    });

    describe('stages', () => {
      const page = DEPLOYMENT_DETAIL_CONFIG.pages.find((p) => p.id === 'ongoing-operations') as
        | ExecutionDetailPageConfig
        | undefined;
      const accessor = page?.tasks.accessor as (data: object) => object[];
      const stateBuilder = page?.tasks.stateBuilder as (
        record: object,
        index: number,
        siblings: object[],
        data: object
      ) => string;

      const condition = (status: number) => ({ status });
      const atStage = (stage: number) => ({ status: { stage } });

      it('marks satisfied conditions as success', () => {
        const conditions = [condition(DEPLOYMENT_CONDITION_STATUS.TRUE)];
        expect(
          stateBuilder(conditions[0], 0, conditions, atStage(DEPLOYMENT_STAGE.PLACEMENT))
        ).toBe(TASK_STATE.SUCCESS);
      });

      it('marks the first incomplete condition as running and later ones as pending during an active rollout', () => {
        const conditions = [
          condition(DEPLOYMENT_CONDITION_STATUS.TRUE),
          condition(DEPLOYMENT_CONDITION_STATUS.FALSE),
          condition(DEPLOYMENT_CONDITION_STATUS.UNKNOWN),
        ];
        const data = atStage(DEPLOYMENT_STAGE.PLACEMENT);

        expect(stateBuilder(conditions[1], 1, conditions, data)).toBe(TASK_STATE.RUNNING);
        expect(stateBuilder(conditions[2], 2, conditions, data)).toBe(TASK_STATE.PENDING);
      });

      it('treats an unknown-status condition as the running step when it is first incomplete', () => {
        const conditions = [
          condition(DEPLOYMENT_CONDITION_STATUS.UNKNOWN),
          condition(DEPLOYMENT_CONDITION_STATUS.FALSE),
        ];
        const data = atStage(DEPLOYMENT_STAGE.VALIDATION);

        expect(stateBuilder(conditions[0], 0, conditions, data)).toBe(TASK_STATE.RUNNING);
        expect(stateBuilder(conditions[1], 1, conditions, data)).toBe(TASK_STATE.PENDING);
      });

      it.each([
        ['rollout failed', DEPLOYMENT_STAGE.ROLLOUT_FAILED],
        ['rollback failed', DEPLOYMENT_STAGE.ROLLBACK_FAILED],
      ])('marks the first incomplete condition as error when %s', (_label, stage) => {
        const conditions = [
          condition(DEPLOYMENT_CONDITION_STATUS.TRUE),
          condition(DEPLOYMENT_CONDITION_STATUS.FALSE),
          condition(DEPLOYMENT_CONDITION_STATUS.UNKNOWN),
        ];
        const data = atStage(stage);

        expect(stateBuilder(conditions[1], 1, conditions, data)).toBe(TASK_STATE.ERROR);
        expect(stateBuilder(conditions[2], 2, conditions, data)).toBe(TASK_STATE.PENDING);
      });

      it('returns live conditions during an active rollout', () => {
        const conditions = [{ type: 'Live' }];
        const conditionsSnapshot = [{ type: 'Snapshot' }];
        expect(
          accessor({
            status: { stage: DEPLOYMENT_STAGE.PLACEMENT, conditions, conditionsSnapshot },
          })
        ).toEqual(conditions);
      });

      it.each([
        ['rollout failed', DEPLOYMENT_STAGE.ROLLOUT_FAILED],
        ['rollback failed', DEPLOYMENT_STAGE.ROLLBACK_FAILED],
      ])('returns the snapshot when %s', (_label, stage) => {
        const conditions = [{ type: 'Live' }];
        const conditionsSnapshot = [{ type: 'Snapshot' }];
        expect(accessor({ status: { stage, conditions, conditionsSnapshot } })).toEqual(
          conditionsSnapshot
        );
      });

      it('falls back to live conditions when the failed-rollout snapshot is empty', () => {
        const conditions = [{ type: 'Live' }];
        expect(
          accessor({
            status: {
              stage: DEPLOYMENT_STAGE.ROLLOUT_FAILED,
              conditions,
              conditionsSnapshot: [],
            },
          })
        ).toEqual(conditions);
      });

      it('returns an empty list when status is missing', () => {
        expect(accessor({})).toEqual([]);
      });
    });
  });
});
