import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { InterpolatableActionsPopover } from '#core/components/actions/interpolatable-actions-popover';
import { RUN_ENTITY_CONFIG } from '#core/config/entities/run/run';
import { PipelineRunState } from '#core/config/entities/run/types';
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

// PhaseEntityConfig.actions is ActionConfigSchema<T>[] where T is the entity's generic
// parameter; InterpolatableActionsPopover expects Data (Record<string, unknown>).
// A PipelineRun is structurally compatible at runtime; cast to unify.
const RETRY_ACTIONS = RUN_ENTITY_CONFIG.actions as ActionConfigSchema<Data>[];

const SOURCE_RUN = 'run-20260818-201315-93e5d0c7';
const NAMESPACE = 'ma-dev-test';
const NEW_RUN = 'run-20260818-999999-newrun';

/**
 * A run as the API returns it — carrying the server-owned metadata and status that must
 * not survive into the retry payload.
 */
function buildFailedRun(overrides: Record<string, unknown> = {}) {
  return {
    typeMeta: { kind: 'PipelineRun', apiVersion: 'michelangelo.api/v2' },
    metadata: {
      name: SOURCE_RUN,
      namespace: NAMESPACE,
      uid: 'c4e05215-7c1b-45bf-89cc-5370c32fc6c7',
      resourceVersion: '5841',
      creationTimestamp: { seconds: '1787084016' },
      finalizers: ['pipelineruns.michelangelo.uber.com/drain'],
      ownerReferences: [{ kind: 'Pipeline', name: 'bert-cola-test' }],
    },
    spec: {
      pipeline: { name: 'bert-cola-test', namespace: NAMESPACE },
      actor: { name: 'Local Developer' },
    },
    status: { state: PipelineRunState.FAILED, workflowId: 'wf-1' },
    ...overrides,
  };
}

async function openRetryDialog(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Actions' }));
  await user.click(await screen.findByRole('option', { name: 'Retry' }));
  return screen.findByRole('dialog', { name: 'Retry Pipeline Run' });
}

/** Finds the payload sent in the (single) CreatePipelineRun call. */
function getCreatePipelineRunPayload(request: ReturnType<typeof createQueryMockRouter>) {
  const createCall = vi.mocked(request).mock.calls.find(([name]) => name === 'CreatePipelineRun');
  expect(createCall).toBeDefined();
  return createCall![1] as {
    typeMeta?: unknown;
    status?: unknown;
    metadata: Record<string, unknown>;
    spec: Record<string, unknown> & {
      actor?: unknown;
      resume?: { pipelineRun?: { name?: string; namespace?: string }; resumeFrom?: string[] };
    };
  };
}

describe('RUN_ENTITY_CONFIG: retry action', () => {
  it('creates a new run that resumes from the run being retried', async () => {
    const user = userEvent.setup();
    const request = createQueryMockRouter({
      CreatePipelineRun: { pipelineRun: { metadata: { name: NEW_RUN, namespace: NAMESPACE } } },
    });
    const record = buildFailedRun();

    render(
      <InterpolatableActionsPopover actions={RETRY_ACTIONS} record={record} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: `/${NAMESPACE}/train/runs/${SOURCE_RUN}` }),
        getServiceProviderWrapper({ request }),
        getSnackbarProviderWrapper(),
      ])
    );

    const dialog = await openRetryDialog(user);
    expect(within(dialog).getByText(/Retry run/)).toHaveTextContent(
      new RegExp(`Retry run ${SOURCE_RUN}`)
    );

    await user.click(within(dialog).getByRole('button', { name: 'Retry' }));

    const payload = await waitFor(() => getCreatePipelineRunPayload(request));

    expect(payload.spec.resume?.pipelineRun).toEqual({
      name: SOURCE_RUN,
      namespace: NAMESPACE,
    });
    // A fresh identity — reusing the source name would collide with the existing run.
    expect(payload.metadata.name).not.toBe(SOURCE_RUN);
    expect(payload.metadata.namespace).toBe(NAMESPACE);
  });

  it('strips the server-owned fields the API rejects or overwrites on create', async () => {
    const user = userEvent.setup();
    const request = createQueryMockRouter({
      CreatePipelineRun: { pipelineRun: { metadata: { name: NEW_RUN, namespace: NAMESPACE } } },
    });
    const record = buildFailedRun();

    render(
      <InterpolatableActionsPopover actions={RETRY_ACTIONS} record={record} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: `/${NAMESPACE}/train/runs/${SOURCE_RUN}` }),
        getServiceProviderWrapper({ request }),
        getSnackbarProviderWrapper(),
      ])
    );

    const dialog = await openRetryDialog(user);
    await user.click(within(dialog).getByRole('button', { name: 'Retry' }));

    const payload = await waitFor(() => getCreatePipelineRunPayload(request));

    expect(payload.status).toBeUndefined();
    expect(payload.typeMeta).toBeUndefined();
    expect(payload.spec.actor).toBeUndefined();
    expect(payload.metadata.uid).toBeUndefined();
    expect(payload.metadata.resourceVersion).toBeUndefined();
    expect(payload.metadata.creationTimestamp).toBeUndefined();
    expect(payload.metadata.finalizers).toBeUndefined();
    expect(payload.metadata.ownerReferences).toBeUndefined();
  });

  it('drops resumeFrom inherited from a run that was itself resumed', async () => {
    const user = userEvent.setup();
    const request = createQueryMockRouter({
      CreatePipelineRun: { pipelineRun: { metadata: { name: NEW_RUN, namespace: NAMESPACE } } },
    });

    // Retrying a resumed run must not re-force the steps that run was asked to re-execute;
    // it should reuse every cached success instead.
    const record = buildFailedRun({
      spec: {
        pipeline: { name: 'bert-cola-test', namespace: NAMESPACE },
        resume: {
          pipelineRun: { name: 'run-older-ancestor', namespace: NAMESPACE },
          resumeFrom: ['load_data'],
        },
      },
    });

    render(
      <InterpolatableActionsPopover actions={RETRY_ACTIONS} record={record} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: `/${NAMESPACE}/train/runs/${SOURCE_RUN}` }),
        getServiceProviderWrapper({ request }),
        getSnackbarProviderWrapper(),
      ])
    );

    const dialog = await openRetryDialog(user);
    await user.click(within(dialog).getByRole('button', { name: 'Retry' }));

    const payload = await waitFor(() => getCreatePipelineRunPayload(request));

    expect(payload.spec.resume?.resumeFrom).toBeUndefined();
    // The new run resumes from the run just retried, not from that run's own ancestor.
    expect(payload.spec.resume?.pipelineRun?.name).toBe(SOURCE_RUN);
  });

  it('confirms with a toast linking to the newly created run', async () => {
    const user = userEvent.setup();
    const request = createQueryMockRouter({
      CreatePipelineRun: { pipelineRun: { metadata: { name: NEW_RUN, namespace: NAMESPACE } } },
    });
    const record = buildFailedRun();

    render(
      <InterpolatableActionsPopover actions={RETRY_ACTIONS} record={record} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: `/${NAMESPACE}/train/runs/${SOURCE_RUN}` }),
        getServiceProviderWrapper({ request }),
        getSnackbarProviderWrapper(),
      ])
    );

    const dialog = await openRetryDialog(user);
    await user.click(within(dialog).getByRole('button', { name: 'Retry' }));

    expect(
      await screen.findByText(
        'A new pipeline run has been created from the failed step of this run.'
      )
    ).toBeInTheDocument();
    // The snackbar mirrors its action into an aria-live region, so the label matches twice.
    expect(screen.getAllByRole('button', { name: 'See new run' })[0]).toBeInTheDocument();
  });

  it.each([
    ['succeeded', PipelineRunState.SUCCEEDED],
    ['running', PipelineRunState.RUNNING],
  ])('disables retry with a tooltip when the run is %s', async (_label, state) => {
    const user = userEvent.setup();
    const record = buildFailedRun({ status: { state } });

    render(
      <InterpolatableActionsPopover actions={RETRY_ACTIONS} record={record} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: `/${NAMESPACE}/train/runs/${SOURCE_RUN}` }),
        getServiceProviderWrapper({ request: vi.fn() }),
        getSnackbarProviderWrapper(),
      ])
    );

    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.hover(await screen.findByRole('option', { name: 'Retry' }));

    expect(
      await screen.findByText('Only failed or killed runs can be retried')
    ).toBeInTheDocument();
  });
});
