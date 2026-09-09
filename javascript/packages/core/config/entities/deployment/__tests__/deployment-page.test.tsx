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

  function buildRetireMockRequest() {
    return createQueryMockRouter({
      UpdateDeployment: {
        deployment: { metadata: { name: DEPLOYMENT_NAME, namespace: NAMESPACE } },
      },
    });
  }

  async function openRetireDialog(user: ReturnType<typeof userEvent.setup>) {
    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Retire' }));
    return screen.findByRole('dialog', {
      name: `Are you sure you want to retire ${DEPLOYMENT_NAME}`,
    });
  }

  function findUpdateDeploymentCall(request: ReturnType<typeof createQueryMockRouter>) {
    return vi.mocked(request).mock.calls.find(([name]) => name === 'UpdateDeployment');
  }

  it('confirms the retire in a dialog, submits the spec with desiredRevision removed, and toasts', async () => {
    const user = userEvent.setup();
    const request = buildRetireMockRequest();

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

    await waitFor(() => expect(findUpdateDeploymentCall(request)).toBeDefined());

    const payload = findUpdateDeploymentCall(request)?.[1] as {
      metadata: { name: string };
      spec: { desiredRevision?: unknown; target?: unknown };
    };
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
    const request = buildRetireMockRequest();

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
    const request = buildRetireMockRequest();

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

    await waitFor(() => expect(findUpdateDeploymentCall(request)).toBeDefined());
  });
});
