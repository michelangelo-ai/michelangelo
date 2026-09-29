import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { InterpolatableActionsPopover } from '#core/components/actions/interpolatable-actions-popover';
import { TASK_STATE } from '#core/components/views/execution/constants';
import { CreateDeploymentForm } from '#core/config/entities/deployment/create-deployment-form';
import { DEPLOYMENT_ENTITY_CONFIG } from '#core/config/entities/deployment/deployment';
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
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getInterpolationProviderWrapper } from '#core/test/wrappers/get-interpolation-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';
import { getSnackbarProviderWrapper } from '#core/test/wrappers/get-snackbar-provider-wrapper';

import type { ActionConfigSchema, Data } from '#core/components/actions/types';
import type { ExecutionDetailPageConfig } from '#core/components/views/detail-view/types/detail-view-schema-types';
import type { DeploymentUpdateInput } from '#core/config/entities/deployment/types';

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
              GetDeployment: {
                deployment: {},
              },
            }),
          }),
        ])
      );

      // The header title comes from the route's entity ID, not deployment.metadata.name.
      expect(screen.getByText('sentiment-deployment')).toBeInTheDocument();
      expect(await screen.findByText('Created')).toBeInTheDocument();
      expect(screen.getByText('Owner')).toBeInTheDocument();
      expect(screen.getByText('Stage')).toBeInTheDocument();
      expect(screen.getByText('State')).toBeInTheDocument();
    });
  });

  describe('information tab', () => {
    it('renders the configuration details', async () => {
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
                  spec: { definition: { type: 1 } },
                },
              },
            }),
          }),
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
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetDeployment: {
                deployment: {
                  spec: {
                    target: { case: 'inferenceServer', value: { name: 'triton-server' } },
                  },
                },
              },
            }),
          }),
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
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetDeployment: {
                deployment: {
                  spec: { desiredRevision: { name: 'sentiment-model-rev-3' } },
                  status: { currentRevision: { name: 'sentiment-model-rev-2' } },
                },
              },
              GetModel: {
                model: {
                  metadata: { creationTimestamp: { seconds: 1746000000 } },
                  spec: {
                    owner: { name: 'model-owner' },
                    kind: 2,
                    sourcePipelineRun: { name: 'run-20260825-080000' },
                  },
                },
              },
            }),
          }),
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
              GetDeployment: {
                deployment: {
                  spec: { desiredRevision: { name: 'sentiment-model-rev-3' } },
                  status: { currentRevision: { name: 'sentiment-model-rev-2' } },
                },
              },
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
                deployment: {},
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

    it('renders state chips matching the task states during an active rollout', async () => {
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
                      { type: 'Validation', status: DEPLOYMENT_CONDITION_STATUS.TRUE },
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
      expect(screen.getByText('Running')).toBeInTheDocument();
      expect(screen.getByText('Pending')).toBeInTheDocument();
      expect(screen.queryByText('Failed')).not.toBeInTheDocument();
    });

    it('renders a "Failed" chip on the first incomplete condition when the rollout failed', async () => {
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
                      { type: 'Validation', status: DEPLOYMENT_CONDITION_STATUS.TRUE },
                      { type: 'Placement', status: DEPLOYMENT_CONDITION_STATUS.FALSE },
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
      expect(screen.getByText('Failed')).toBeInTheDocument();
      expect(screen.getByText('Pending')).toBeInTheDocument();
      expect(screen.queryByText('Running')).not.toBeInTheDocument();
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

      it('returns the snapshot when the rollout has failed', () => {
        const conditions = [{ type: 'Live' }];
        const conditionsSnapshot = [{ type: 'Snapshot' }];
        expect(
          accessor({
            status: { stage: DEPLOYMENT_STAGE.ROLLOUT_FAILED, conditions, conditionsSnapshot },
          })
        ).toEqual(conditionsSnapshot);
      });

      it('returns live conditions when the rollback has failed, even if a snapshot exists', () => {
        const conditions = [{ type: 'Live' }];
        const conditionsSnapshot = [{ type: 'Snapshot' }];
        expect(
          accessor({
            status: { stage: DEPLOYMENT_STAGE.ROLLBACK_FAILED, conditions, conditionsSnapshot },
          })
        ).toEqual(conditions);
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

describe('Deployment retire action', () => {
  const RETIRE_ACTIONS = DEPLOYMENT_ENTITY_CONFIG.actions as ActionConfigSchema<Data>[];

  const DEPLOYMENT_NAME = 'test-retire-action';
  const NAMESPACE = 'ma-dev-test';

  function buildDeployedRecord(overrides: Record<string, unknown> = {}) {
    return {
      metadata: {
        name: DEPLOYMENT_NAME,
        namespace: NAMESPACE,
        creationTimestamp: { seconds: 1757019547 },
      },
      spec: {
        desiredRevision: { name: 'bert-cola-37', namespace: NAMESPACE },
        target: { case: 'inferenceServer', value: { name: 'inference-server-example' } },
      },
      status: {
        currentRevision: { name: 'bert-cola-37', namespace: NAMESPACE },
      },
      ...overrides,
    };
  }

  async function openRetireDialog(user: ReturnType<typeof userEvent.setup>) {
    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Retire' }));
    return screen.findByRole('dialog', {
      name: `Are you sure you want to retire ${DEPLOYMENT_NAME}`,
    });
  }

  it('confirms the retire in a dialog, submits the spec with desiredRevision removed, and toasts', async () => {
    const user = userEvent.setup();
    const request = createQueryMockRouter({
      UpdateDeployment: {
        deployment: { metadata: { name: DEPLOYMENT_NAME, namespace: NAMESPACE } },
      },
    });

    render(
      <InterpolatableActionsPopover actions={RETIRE_ACTIONS} record={buildDeployedRecord()} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: `/${NAMESPACE}/deploy/deployments/${DEPLOYMENT_NAME}` }),
        getServiceProviderWrapper({ request }),
        getSnackbarProviderWrapper(),
      ])
    );

    const dialog = await openRetireDialog(user);
    expect(within(dialog).getByText(/Deployed at:/)).toBeInTheDocument();
    expect(within(dialog).getByText('This process might take a few minutes.')).toBeInTheDocument();

    await user.click(within(dialog).getByRole('button', { name: 'Yes, retire' }));

    await waitFor(() => expect(request.getCall('UpdateDeployment')).toBeDefined());

    // cast: recorded call args are `unknown`; DeploymentUpdateInput is what this test's request actually sends
    const payload = request.getCall('UpdateDeployment')!.args as DeploymentUpdateInput;
    // The absent desiredRevision is what tells the backend to run cleanup; the rest of
    // the spec must be sent through intact.
    expect(payload.spec.desiredRevision).toBeUndefined();
    expect(payload.spec.target).toEqual({
      case: 'inferenceServer',
      value: { name: 'inference-server-example' },
    });
    expect(payload.metadata.name).toBe(DEPLOYMENT_NAME);

    expect(
      await screen.findByText(`Retirement for deployment ${DEPLOYMENT_NAME} has begun`)
    ).toBeInTheDocument();
  });

  it('disables retire with a tooltip when the deployment has no revision to retire', async () => {
    const user = userEvent.setup();
    const request = createQueryMockRouter({
      UpdateDeployment: {
        deployment: { metadata: { name: DEPLOYMENT_NAME, namespace: NAMESPACE } },
      },
    });

    const record = buildDeployedRecord({
      spec: { target: { case: 'inferenceServer', value: { name: 'inference-server-example' } } },
      status: {},
    });

    render(
      <InterpolatableActionsPopover actions={RETIRE_ACTIONS} record={record} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: `/${NAMESPACE}/deploy/deployments/${DEPLOYMENT_NAME}` }),
        getServiceProviderWrapper({ request }),
        getSnackbarProviderWrapper(),
      ])
    );

    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.hover(await screen.findByRole('option', { name: 'Retire' }));
    expect(await screen.findByText('Deployment has already been retired')).toBeInTheDocument();

    await user.click(screen.getByRole('option', { name: 'Retire' }));
    expect(
      screen.queryByRole('dialog', { name: `Are you sure you want to retire ${DEPLOYMENT_NAME}` })
    ).not.toBeInTheDocument();
    expect(request).not.toHaveBeenCalled();
  });

  it('stays enabled while a candidate revision is still rolling out', async () => {
    const user = userEvent.setup();
    const request = createQueryMockRouter({
      UpdateDeployment: {
        deployment: { metadata: { name: DEPLOYMENT_NAME, namespace: NAMESPACE } },
      },
    });

    // desiredRevision already cleared but a candidate is mid-rollout — retiring must
    // still be possible to abort the rollout, matching the backend's cleanup trigger.
    const record = buildDeployedRecord({
      spec: { target: { case: 'inferenceServer', value: { name: 'inference-server-example' } } },
      status: { candidateRevision: { name: 'bert-cola-37', namespace: NAMESPACE } },
    });

    render(
      <InterpolatableActionsPopover actions={RETIRE_ACTIONS} record={record} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: `/${NAMESPACE}/deploy/deployments/${DEPLOYMENT_NAME}` }),
        getServiceProviderWrapper({ request }),
        getSnackbarProviderWrapper(),
      ])
    );

    const dialog = await openRetireDialog(user);
    await user.click(within(dialog).getByRole('button', { name: 'Yes, retire' }));

    await waitFor(() => expect(request.getCall('UpdateDeployment')).toBeDefined());
  });
});

describe('Deployment delete action', () => {
  const DEPLOYMENT_ACTIONS = DEPLOYMENT_ENTITY_CONFIG.actions as ActionConfigSchema<Data>[];

  const DEPLOYMENT_NAME = 'test-delete-action';
  const NAMESPACE = 'ma-dev-test';

  async function openDeleteDialog(user: ReturnType<typeof userEvent.setup>) {
    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Delete' }));
    return screen.findByRole('dialog', {
      name: `Are you sure you want to delete “${DEPLOYMENT_NAME}” ?`,
    });
  }

  function findDeleteDeploymentCall(request: ReturnType<typeof createQueryMockRouter>) {
    return vi.mocked(request).mock.calls.find(([name]) => name === 'DeleteDeployment');
  }

  it('warns in the dialog, sends nothing on cancel, then deletes the record and toasts on confirm', async () => {
    const user = userEvent.setup();
    const request = createQueryMockRouter({ DeleteDeployment: {} });
    const record = {
      metadata: {
        name: DEPLOYMENT_NAME,
        namespace: NAMESPACE,
        creationTimestamp: { seconds: 1757019547 },
      },
      spec: {
        desiredRevision: { name: 'bert-cola-37', namespace: NAMESPACE },
        target: { case: 'inferenceServer', value: { name: 'inference-server-example' } },
      },
      status: {},
    };

    render(
      <InterpolatableActionsPopover actions={DEPLOYMENT_ACTIONS} record={record} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: `/${NAMESPACE}/deploy/deployments/${DEPLOYMENT_NAME}` }),
        getServiceProviderWrapper({ request }),
        getSnackbarProviderWrapper(),
      ])
    );

    let dialog = await openDeleteDialog(user);
    expect(
      within(dialog).getByText(
        'We will perform retirement process first and then the deployment will be deleted. This process will take few minutes to complete.'
      )
    ).toBeInTheDocument();
    expect(
      within(dialog).getByText(
        'If there are any online existing prediction requests or offline pipeline runs in this deployment this call will fail.'
      )
    ).toBeInTheDocument();

    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(findDeleteDeploymentCall(request)).toBeUndefined();

    dialog = await openDeleteDialog(user);
    await user.click(within(dialog).getByRole('button', { name: 'Yes, delete' }));

    await waitFor(() => expect(findDeleteDeploymentCall(request)).toBeDefined());

    // The handler reshapes the record into { name, namespace }; the action itself
    // submits the record unchanged.
    const payload = findDeleteDeploymentCall(request)?.[1] as {
      metadata: { name: string; namespace: string };
    };
    expect(payload.metadata.name).toBe(DEPLOYMENT_NAME);
    expect(payload.metadata.namespace).toBe(NAMESPACE);

    expect(
      await screen.findByText('Deployment has been deleted. This process may take a few seconds.')
    ).toBeInTheDocument();
  });
});

describe('Deployment update action', () => {
  const DEPLOYMENT_ACTIONS = DEPLOYMENT_ENTITY_CONFIG.actions as ActionConfigSchema<Data>[];

  const DEPLOYMENT_NAME = 'test-update-action';
  const NAMESPACE = 'ma-dev-test';

  async function openUpdateDialog(user: ReturnType<typeof userEvent.setup>) {
    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Update deployment' }));
    return screen.findByRole('dialog', { name: 'Update deployment' });
  }

  /** Finds the payload sent in the (single) UpdateDeployment call. */
  function getUpdateDeploymentPayload(request: ReturnType<typeof createQueryMockRouter>) {
    const updateCall = vi.mocked(request).mock.calls.find(([name]) => name === 'UpdateDeployment');
    expect(updateCall).toBeDefined();
    return updateCall![1] as {
      metadata: { name: string };
      spec: {
        desiredRevision?: { name?: string };
        strategy?: { rolloutStrategy?: { case?: string } };
        target?: { value?: { name?: string } };
        modelFamily?: { name?: string };
      };
      status?: unknown;
    };
  }

  it('opens prefilled with name, inference server, and model family read-only', async () => {
    const user = userEvent.setup();
    const request = createQueryMockRouter({
      ListInferenceServer: {
        inferenceServerList: { items: [{ metadata: { name: 'inference-server-example' } }] },
      },
      ListModelFamily: {
        modelFamilyList: {
          items: [{ metadata: { name: 'bert-cola' }, spec: { name: 'bert-cola' } }],
        },
      },
      ListModel: {
        modelList: {
          items: [{ metadata: { name: 'bert-cola-37' } }, { metadata: { name: 'bert-cola-38' } }],
        },
      },
    });

    render(
      <InterpolatableActionsPopover
        actions={DEPLOYMENT_ACTIONS}
        record={{
          metadata: { name: DEPLOYMENT_NAME, namespace: NAMESPACE },
          spec: {
            desiredRevision: { name: 'bert-cola-37', namespace: NAMESPACE },
            target: { case: 'inferenceServer', value: { name: 'inference-server-example' } },
            modelFamily: { name: 'bert-cola', namespace: NAMESPACE },
          },
        }}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: `/${NAMESPACE}/deploy/deployments/${DEPLOYMENT_NAME}` }),
        getServiceProviderWrapper({ request }),
        getSnackbarProviderWrapper(),
      ])
    );

    const dialog = await openUpdateDialog(user);

    const nameInput = within(dialog).getByRole('textbox', { name: 'Name *' });
    expect(nameInput).toHaveValue(DEPLOYMENT_NAME);
    expect(nameInput).toHaveAttribute('readonly');

    // Prefilled selects' accessible names are their selected values.
    const serverSelect = await within(dialog).findByRole('combobox', {
      name: /Selected inference-server-example\./,
    });
    expect(serverSelect).toHaveAttribute('readonly');

    // The family comes from record.spec.modelFamily and is locked in update mode.
    const familySelect = await within(dialog).findByRole('combobox', {
      name: /Selected bert-cola\./,
    });
    expect(familySelect).toHaveAttribute('readonly');

    expect(await within(dialog).findByText('bert-cola-37')).toBeInTheDocument();
  });

  it('submits the full record with the newly selected model as desiredRevision', async () => {
    const user = userEvent.setup();
    const request = createQueryMockRouter({
      UpdateDeployment: {
        deployment: { metadata: { name: DEPLOYMENT_NAME, namespace: NAMESPACE } },
      },
      ListInferenceServer: {
        inferenceServerList: { items: [{ metadata: { name: 'inference-server-example' } }] },
      },
      ListModelFamily: {
        modelFamilyList: {
          items: [{ metadata: { name: 'bert-cola' }, spec: { name: 'bert-cola' } }],
        },
      },
      ListModel: {
        modelList: {
          items: [{ metadata: { name: 'bert-cola-37' } }, { metadata: { name: 'bert-cola-38' } }],
        },
      },
    });

    render(
      <InterpolatableActionsPopover
        actions={DEPLOYMENT_ACTIONS}
        record={{
          metadata: { name: DEPLOYMENT_NAME, namespace: NAMESPACE },
          spec: {
            desiredRevision: { name: 'bert-cola-37', namespace: NAMESPACE },
            target: { case: 'inferenceServer', value: { name: 'inference-server-example' } },
            strategy: { rolloutStrategy: { case: 'rolling', value: { incrementPercentage: 10 } } },
            definition: { type: 1 },
            modelFamily: { name: 'bert-cola', namespace: NAMESPACE },
          },
          status: { currentRevision: { name: 'bert-cola-37', namespace: NAMESPACE } },
        }}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: `/${NAMESPACE}/deploy/deployments/${DEPLOYMENT_NAME}` }),
        getServiceProviderWrapper({ request }),
        getSnackbarProviderWrapper(),
      ])
    );

    const dialog = await openUpdateDialog(user);

    // The prefilled Model select's accessible name is its selected value.
    await user.click(within(dialog).getByRole('combobox', { name: /Selected bert-cola-37/ }));
    await user.click(await screen.findByRole('option', { name: 'bert-cola-38' }));
    await user.click(within(dialog).getByRole('button', { name: 'Update' }));

    const payload = await waitFor(() => getUpdateDeploymentPayload(request));

    expect(payload.spec.desiredRevision?.name).toBe('bert-cola-38');
    // Everything else on the record rides along unchanged.
    expect(payload.metadata.name).toBe(DEPLOYMENT_NAME);
    expect(payload.spec.target?.value?.name).toBe('inference-server-example');
    expect(payload.spec.strategy?.rolloutStrategy?.case).toBe('rolling');
    expect(payload.spec.modelFamily?.name).toBe('bert-cola');
    expect(payload.status).toBeDefined();
  });
});

describe('Deployment create action', () => {
  it('fills in every field, submits the deployment, and toasts', async () => {
    const user = userEvent.setup();
    const request = createQueryMockRouter({
      CreateDeployment: { deployment: { metadata: { name: 'new-deployment' } } },
      ListInferenceServer: {
        inferenceServerList: { items: [{ metadata: { name: 'inference-server-example' } }] },
      },
      ListModelFamily: {
        modelFamilyList: {
          items: [{ metadata: { name: 'bert-cola' }, spec: { name: 'bert-cola' } }],
        },
      },
      ListModel: {
        modelList: { items: [{ metadata: { name: 'bert-cola-40' } }] },
      },
    });

    render(
      <CreateDeploymentForm onClose={vi.fn()} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/ma-dev-test/deploy/deployments' }),
        getServiceProviderWrapper({ request }),
        getSnackbarProviderWrapper(),
      ])
    );

    const dialog = await screen.findByRole('dialog', { name: 'Create deployment' });
    await user.type(within(dialog).getByRole('textbox', { name: 'Name *' }), 'new-deployment');

    await user.click(within(dialog).getByRole('combobox', { name: 'Inference server *' }));
    await user.click(await screen.findByRole('option', { name: 'inference-server-example' }));

    await user.click(within(dialog).getByRole('combobox', { name: 'Model family' }));
    await user.click(await screen.findByRole('option', { name: 'bert-cola' }));

    await user.click(await within(dialog).findByRole('combobox', { name: 'Model *' }));
    await user.click(await screen.findByRole('option', { name: 'bert-cola-40' }));

    await user.click(within(dialog).getByRole('button', { name: 'Create' }));

    await waitFor(() => {
      expect(request).toHaveBeenCalledWith(
        'CreateDeployment',
        {
          metadata: { name: 'new-deployment', namespace: 'ma-dev-test' },
          spec: {
            modelFamily: { name: 'bert-cola', namespace: 'ma-dev-test' },
            desiredRevision: { name: 'bert-cola-40', namespace: 'ma-dev-test' },
            target: {
              case: 'inferenceServer',
              value: { name: 'inference-server-example', namespace: 'ma-dev-test' },
            },
            strategy: { rolloutStrategy: { case: 'rolling', value: { incrementPercentage: 0 } } },
            definition: { type: 1 },
          },
        },
        {}
      );
    });

    expect(await screen.findByText('Deployment created')).toBeInTheDocument();
  });
});
