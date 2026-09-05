import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { InterpolatableActionsPopover } from '#core/components/actions/interpolatable-actions-popover';
import { DEPLOYMENT_ENTITY_CONFIG } from '#core/config/entities/deployment/deployment';
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

  function buildRequestCapture() {
    const submitted: Record<string, unknown>[] = [];
    const request = (name: string, payload: unknown) => {
      if (name === 'UpdateDeployment') {
        submitted.push(payload as Record<string, unknown>);
        return Promise.resolve({
          deployment: { metadata: { name: DEPLOYMENT_NAME, namespace: NAMESPACE } },
        });
      }
      return Promise.resolve({});
    };
    return { submitted, request };
  }

  async function openRetireDialog(user: ReturnType<typeof userEvent.setup>) {
    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Retire' }));
    return screen.findByRole('dialog', {
      name: `Are you sure you want to retire ${DEPLOYMENT_NAME}`,
    });
  }

  it('updates the deployment with desiredRevision removed, leaving the rest of the spec intact', async () => {
    const user = userEvent.setup();
    const { submitted, request } = buildRequestCapture();

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
    await user.click(within(dialog).getByRole('button', { name: 'Yes, retire' }));

    await waitFor(() => expect(submitted).toHaveLength(1));

    const payload = submitted[0] as {
      metadata: { name: string };
      spec: { desiredRevision?: unknown; target?: unknown };
    };
    // The absent desiredRevision is what tells the backend to run cleanup.
    expect(payload.spec.desiredRevision).toBeUndefined();
    expect(payload.spec.target).toEqual({
      case: 'inferenceServer',
      value: { name: 'inference-server-example' },
    });
    expect(payload.metadata.name).toBe(DEPLOYMENT_NAME);
  });

  it('shows the deployed/last-used timestamps in the confirm dialog', async () => {
    const user = userEvent.setup();
    const { request } = buildRequestCapture();

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
    // No last-prediction annotation on the record → N/A.
    expect(within(dialog).getByText(/Deployed at:/)).toBeInTheDocument();
    expect(within(dialog).getByText(/Last used at:/)).toBeInTheDocument();
    expect(within(dialog).getByText('N/A')).toBeInTheDocument();
    expect(within(dialog).getByText('This process might take a few minutes.')).toBeInTheDocument();
  });

  it('confirms with a toast naming the deployment being retired', async () => {
    const user = userEvent.setup();
    const { request } = buildRequestCapture();

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
    await user.click(within(dialog).getByRole('button', { name: 'Yes, retire' }));

    expect(
      await screen.findByText(`Retirement for deployment ${DEPLOYMENT_NAME} has begun`)
    ).toBeInTheDocument();
  });

  it('disables retire with a tooltip when the deployment has no revision to retire', async () => {
    const user = userEvent.setup();
    const request = vi.fn();

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
    const { submitted, request } = buildRequestCapture();

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

    await waitFor(() => expect(submitted).toHaveLength(1));
  });
});

describe('Deployment delete action', () => {
  const DEPLOYMENT_ACTIONS = DEPLOYMENT_ENTITY_CONFIG.actions as ActionConfigSchema<Data>[];

  const DEPLOYMENT_NAME = 'test-delete-action';
  const NAMESPACE = 'ma-dev-test';

  function buildRecord() {
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
      status: {},
    };
  }

  function buildRequestCapture() {
    const submitted: Record<string, unknown>[] = [];
    const request = (name: string, payload: unknown) => {
      if (name === 'DeleteDeployment') {
        submitted.push(payload as Record<string, unknown>);
      }
      return Promise.resolve({});
    };
    return { submitted, request };
  }

  async function openDeleteDialog(user: ReturnType<typeof userEvent.setup>) {
    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Delete' }));
    return screen.findByRole('dialog', {
      name: `Are you sure you want to delete “${DEPLOYMENT_NAME}” ?`,
    });
  }

  it('sends the record to DeleteDeployment and confirms with a toast', async () => {
    const user = userEvent.setup();
    const { submitted, request } = buildRequestCapture();

    render(
      <InterpolatableActionsPopover actions={DEPLOYMENT_ACTIONS} record={buildRecord()} />,
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

    const dialog = await openDeleteDialog(user);
    await user.click(within(dialog).getByRole('button', { name: 'Yes, delete' }));

    await waitFor(() => expect(submitted).toHaveLength(1));

    // The handler reshapes the record into { name, namespace }; the action itself
    // submits the record unchanged.
    const payload = submitted[0] as { metadata: { name: string; namespace: string } };
    expect(payload.metadata.name).toBe(DEPLOYMENT_NAME);
    expect(payload.metadata.namespace).toBe(NAMESPACE);

    expect(
      await screen.findByText('Deployment has been deleted. This process may take a few seconds.')
    ).toBeInTheDocument();
  });

  it('warns that retirement runs first and in-flight traffic fails the call', async () => {
    const user = userEvent.setup();
    const { submitted, request } = buildRequestCapture();

    render(
      <InterpolatableActionsPopover actions={DEPLOYMENT_ACTIONS} record={buildRecord()} />,
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

    const dialog = await openDeleteDialog(user);
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
    expect(submitted).toHaveLength(0);
  });
});

describe('Deployment update action', () => {
  const DEPLOYMENT_ACTIONS = DEPLOYMENT_ENTITY_CONFIG.actions as ActionConfigSchema<Data>[];

  const DEPLOYMENT_NAME = 'test-update-action';
  const NAMESPACE = 'ma-dev-test';

  function buildRecord() {
    return {
      metadata: {
        name: DEPLOYMENT_NAME,
        namespace: NAMESPACE,
        creationTimestamp: { seconds: 1757019547 },
      },
      spec: {
        desiredRevision: { name: 'bert-cola-37', namespace: NAMESPACE },
        target: { case: 'inferenceServer', value: { name: 'inference-server-example' } },
        strategy: { rolloutStrategy: { case: 'rolling', value: { incrementPercentage: 10 } } },
        definition: { type: 1 },
      },
      status: { currentRevision: { name: 'bert-cola-37', namespace: NAMESPACE } },
    };
  }

  function buildRequestCapture() {
    const submitted: Record<string, unknown>[] = [];
    const request = (name: string, payload: unknown) => {
      switch (name) {
        case 'UpdateDeployment':
          submitted.push(payload as Record<string, unknown>);
          return Promise.resolve({
            deployment: { metadata: { name: DEPLOYMENT_NAME, namespace: NAMESPACE } },
          });
        case 'GetModel':
          return Promise.resolve({
            model: { spec: { modelFamily: { name: 'bert-cola' } } },
          });
        case 'ListInferenceServer':
          return Promise.resolve({
            inferenceServerList: { items: [{ metadata: { name: 'inference-server-example' } }] },
          });
        case 'ListModelFamily':
          return Promise.resolve({
            modelFamilyList: {
              items: [{ metadata: { name: 'bert-cola' }, spec: { name: 'bert-cola' } }],
            },
          });
        case 'ListModel':
          return Promise.resolve({
            modelList: {
              items: [
                { metadata: { name: 'bert-cola-37' } },
                { metadata: { name: 'bert-cola-38' } },
              ],
            },
          });
        default:
          return Promise.resolve({});
      }
    };
    return { submitted, request };
  }

  async function openUpdateDialog(user: ReturnType<typeof userEvent.setup>) {
    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Update deployment' }));
    return screen.findByRole('dialog', { name: 'Update deployment' });
  }

  it('lists Update deployment as the first action in the menu', async () => {
    const user = userEvent.setup();
    const { request } = buildRequestCapture();

    render(
      <InterpolatableActionsPopover actions={DEPLOYMENT_ACTIONS} record={buildRecord()} />,
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
    const options = await screen.findAllByRole('option');
    expect(options[0]).toHaveTextContent('Update deployment');
  });

  it('opens prefilled with name and inference server read-only', async () => {
    const user = userEvent.setup();
    const { request } = buildRequestCapture();

    render(
      <InterpolatableActionsPopover actions={DEPLOYMENT_ACTIONS} record={buildRecord()} />,
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

    // Prefilled selects render their values as text within the dialog.
    expect(await within(dialog).findByText('inference-server-example')).toBeInTheDocument();
    expect(await within(dialog).findByText('bert-cola')).toBeInTheDocument();
    expect(await within(dialog).findByText('bert-cola-37')).toBeInTheDocument();
  });

  it('locks the model family so only the model can be changed', async () => {
    const user = userEvent.setup();
    const { request } = buildRequestCapture();

    render(
      <InterpolatableActionsPopover actions={DEPLOYMENT_ACTIONS} record={buildRecord()} />,
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

    // The prefilled selects' accessible names are their selected values.
    const familySelect = await within(dialog).findByRole('combobox', {
      name: /Selected bert-cola\./,
    });
    expect(familySelect).toHaveAttribute('readonly');
    // Clicking a read-only select must not open its dropdown.
    await user.click(familySelect);
    expect(screen.queryByRole('option')).not.toBeInTheDocument();

    // The model select stays editable.
    const modelSelect = within(dialog).getByRole('combobox', { name: /Selected bert-cola-37/ });
    expect(modelSelect).not.toHaveAttribute('readonly');
    await user.click(modelSelect);
    expect(await screen.findByRole('option', { name: 'bert-cola-38' })).toBeInTheDocument();
  });

  it('submits the full record with the newly selected model as desiredRevision', async () => {
    const user = userEvent.setup();
    const { submitted, request } = buildRequestCapture();

    render(
      <InterpolatableActionsPopover actions={DEPLOYMENT_ACTIONS} record={buildRecord()} />,
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

    await waitFor(() => expect(submitted).toHaveLength(1));

    const payload = submitted[0] as {
      metadata: { name: string };
      spec: {
        desiredRevision?: { name?: string };
        strategy?: { rolloutStrategy?: { case?: string } };
        target?: { value?: { name?: string } };
      };
      status?: unknown;
    };
    expect(payload.spec.desiredRevision?.name).toBe('bert-cola-38');
    // Everything else on the record rides along unchanged.
    expect(payload.metadata.name).toBe(DEPLOYMENT_NAME);
    expect(payload.spec.target?.value?.name).toBe('inference-server-example');
    expect(payload.spec.strategy?.rolloutStrategy?.case).toBe('rolling');
    expect(payload.status).toBeDefined();
  });
});
