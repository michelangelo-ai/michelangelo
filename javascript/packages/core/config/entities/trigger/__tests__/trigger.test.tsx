import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { DetailViewHeader } from '#core/components/views/detail-view/components/detail-view-header/detail-view-header';
import { TRIGGER_ENTITY_CONFIG } from '#core/config/entities/trigger/trigger';
import { TriggerRunAction, TriggerRunState } from '#core/config/entities/trigger/types';
import { TRAIN_PHASE } from '#core/config/phases/train';
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
import { getUserProviderWrapper } from '#core/test/wrappers/get-user-provider-wrapper';

import type { ActionConfigSchema, Data } from '#core/components/actions/types';
import type { TriggerRun } from '#core/config/entities/trigger/types';
import type { ServiceContextType } from '#core/providers/service-provider/types';

// PhaseEntityConfig.actions is ActionConfigSchema<T>[] where T is the entity's
// generic parameter; DetailViewHeader expects ActionConfigSchema<object>[] (Data).
// TriggerRun is structurally compatible at runtime; cast to unify.
const TRIGGER_ACTIONS = TRIGGER_ENTITY_CONFIG.actions as ActionConfigSchema<Data>[];

function buildRunningTriggerRun(overrides: Partial<TriggerRun> = {}): TriggerRun {
  return {
    metadata: { name: 'my-trigger', namespace: 'test-ns' },
    spec: {
      pipeline: { name: 'my-pipeline', namespace: 'test-ns' },
      revision: { name: 'rev-1', namespace: 'test-ns' },
      actor: { name: 'me' },
      sourceTriggerName: '',
      autoFlip: false,
      notifications: [],
      kill: false,
      action: TriggerRunAction.NO_ACTION,
    },
    status: { state: TriggerRunState.RUNNING },
    ...overrides,
  };
}

describe('TRIGGER_ENTITY_CONFIG: kill action', () => {
  it('renders Kill as a top-level header button, not tucked into an overflow menu', async () => {
    const record = buildRunningTriggerRun();

    render(
      <DetailViewHeader title="my-trigger" actions={TRIGGER_ACTIONS} record={record} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/test-ns/triggers' }),
        getServiceProviderWrapper({ request: vi.fn() }),
        getSnackbarProviderWrapper(),
      ])
    );

    expect(await screen.findByRole('button', { name: 'Kill' })).toBeInTheDocument();
    // No overflow trigger should be rendered — Kill is the only action, and it's
    // always shown directly rather than collapsing into a "..." popover.
    expect(screen.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument();
  });

  it('opens a confirm dialog naming the run and pipeline, fires UpdateTriggerRun with spec.action=KILL', async () => {
    const user = userEvent.setup();
    const record = buildRunningTriggerRun();
    const mockRequest = createQueryMockRouter({ UpdateTriggerRun: { triggerRun: record } });

    render(
      <DetailViewHeader title="my-trigger" actions={TRIGGER_ACTIONS} record={record} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/test-ns/triggers' }),
        getServiceProviderWrapper({ request: mockRequest }),
        getSnackbarProviderWrapper(),
      ])
    );

    await user.click(await screen.findByRole('button', { name: 'Kill' }));

    const dialog = await screen.findByRole('dialog', { name: 'Kill Trigger Run' });
    expect(within(dialog).getByText(/Kill run/)).toHaveTextContent(
      /Kill run my-trigger in pipeline my-pipeline/
    );

    await user.click(within(dialog).getByRole('button', { name: 'Kill' }));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'UpdateTriggerRun',
        expect.objectContaining({
          spec: expect.objectContaining({
            action: TriggerRunAction.KILL,
          }) as Record<string, unknown>,
        }),
        {}
      );
    });
  });

  it('is enabled when the run is running', async () => {
    const record = buildRunningTriggerRun({ status: { state: TriggerRunState.RUNNING } });

    render(
      <DetailViewHeader title="my-trigger" actions={TRIGGER_ACTIONS} record={record} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/test-ns/triggers' }),
        getServiceProviderWrapper({ request: vi.fn() }),
        getSnackbarProviderWrapper(),
      ])
    );

    expect(await screen.findByRole('button', { name: 'Kill' })).toBeEnabled();
  });

  it('disables the action with a tooltip when the run is not killable', async () => {
    const user = userEvent.setup();
    const record = buildRunningTriggerRun({ status: { state: TriggerRunState.SUCCEEDED } });

    render(
      <DetailViewHeader title="my-trigger" actions={TRIGGER_ACTIONS} record={record} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/test-ns/triggers' }),
        getServiceProviderWrapper({ request: vi.fn() }),
        getSnackbarProviderWrapper(),
      ])
    );

    const killButton = await screen.findByRole('button', { name: 'Kill' });
    expect(killButton).toBeDisabled();

    await user.hover(killButton);
    expect(
      await screen.findByText('Only running or paused trigger runs can be killed')
    ).toBeInTheDocument();
  });

  it('keeps dialog open and shows the error when the mutation fails', async () => {
    const user = userEvent.setup();
    const record = buildRunningTriggerRun();
    const mockRequest = createQueryMockRouter({ UpdateTriggerRun: new Error('Kill failed') });

    render(
      <DetailViewHeader title="my-trigger" actions={TRIGGER_ACTIONS} record={record} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getIconProviderWrapper(),
        getInterpolationProviderWrapper(),
        getRouterWrapper({ location: '/test-ns/triggers' }),
        getServiceProviderWrapper({ request: mockRequest }),
        getSnackbarProviderWrapper(),
      ])
    );

    await user.click(await screen.findByRole('button', { name: 'Kill' }));
    const dialog = await screen.findByRole('dialog', { name: 'Kill Trigger Run' });
    await user.click(within(dialog).getByRole('button', { name: 'Kill' }));

    await within(dialog).findByText(/Test error/);
    expect(screen.getByRole('dialog', { name: 'Kill Trigger Run' })).toBeInTheDocument();
  });
});

describe('TRIGGER_ENTITY_CONFIG: rerun action', () => {
  function buildTerminalTriggerRun(overrides: Partial<TriggerRun> = {}): TriggerRun {
    return buildRunningTriggerRun({
      status: { state: TriggerRunState.FAILED },
      spec: {
        pipeline: { name: 'my-pipeline', namespace: 'test-ns' },
        revision: { name: 'rev-1', namespace: 'test-ns' },
        actor: { name: 'me' },
        trigger: { triggerType: { case: 'cronSchedule', value: { cron: '0 * * * *' } } },
        sourceTriggerName: 'nightly',
        autoFlip: false,
        notifications: [],
        kill: false,
        action: TriggerRunAction.NO_ACTION,
      },
      ...overrides,
    });
  }

  function buildRerunWrappers(request: ServiceContextType['request']) {
    return [
      getBaseProviderWrapper(),
      getErrorProviderWrapper(),
      getIconProviderWrapper(),
      getInterpolationProviderWrapper(),
      getRouterWrapper({ location: '/test-ns/triggers' }),
      getSnackbarProviderWrapper(),
      getServiceProviderWrapper({ request }),
    ];
  }

  it('renders Kill and Rerun as separate, always-visible header buttons with no overflow menu', async () => {
    const record = buildTerminalTriggerRun();

    render(
      <DetailViewHeader title="my-trigger" actions={TRIGGER_ACTIONS} record={record} />,
      buildWrapper(buildRerunWrappers(vi.fn()))
    );

    // Kill has a static primary hierarchy and Rerun a static secondary hierarchy, so both
    // always render as their own header buttons; neither is ever tertiary, so the "..."
    // overflow popover never renders.
    expect(await screen.findByRole('button', { name: 'Kill' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Rerun' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument();
  });

  it("has Kill and Rerun's enabled state respond independently to the run's own status", async () => {
    const runningRecord = buildTerminalTriggerRun({ status: { state: TriggerRunState.RUNNING } });

    const { unmount } = render(
      <DetailViewHeader title="my-trigger" actions={TRIGGER_ACTIONS} record={runningRecord} />,
      buildWrapper(buildRerunWrappers(vi.fn()))
    );

    // Running: killable, not yet terminated, so Kill is enabled and Rerun is disabled.
    expect(await screen.findByRole('button', { name: 'Kill' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Rerun' })).toBeDisabled();
    unmount();

    const failedRecord = buildTerminalTriggerRun({ status: { state: TriggerRunState.FAILED } });

    render(
      <DetailViewHeader title="my-trigger" actions={TRIGGER_ACTIONS} record={failedRecord} />,
      buildWrapper(buildRerunWrappers(vi.fn()))
    );

    // Failed: no longer killable, but terminated, so Kill is disabled and Rerun is enabled.
    expect(await screen.findByRole('button', { name: 'Kill' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Rerun' })).toBeEnabled();
  });

  it('disables Rerun with a tooltip when the trigger run has not terminated', async () => {
    const user = userEvent.setup();
    const record = buildTerminalTriggerRun({ status: { state: TriggerRunState.RUNNING } });

    render(
      <DetailViewHeader title="my-trigger" actions={TRIGGER_ACTIONS} record={record} />,
      buildWrapper(buildRerunWrappers(vi.fn()))
    );

    const rerunButton = await screen.findByRole('button', { name: 'Rerun' });
    expect(rerunButton).toBeDisabled();

    await user.hover(rerunButton);
    expect(
      await screen.findByText(
        'Only terminated trigger runs (failed, killed, or succeeded) can be rerun'
      )
    ).toBeInTheDocument();
  });

  it.each([TriggerRunState.FAILED, TriggerRunState.KILLED, TriggerRunState.SUCCEEDED])(
    'enables Rerun when the trigger run state is terminal (%i)',
    async (state) => {
      const record = buildTerminalTriggerRun({ status: { state } });

      render(
        <DetailViewHeader title="my-trigger" actions={TRIGGER_ACTIONS} record={record} />,
        buildWrapper(buildRerunWrappers(vi.fn()))
      );

      expect(await screen.findByRole('button', { name: 'Rerun' })).toBeEnabled();
    }
  );

  it('creates a new TriggerRun copying pipeline/revision/schedule and clearing kill state', async () => {
    const user = userEvent.setup();
    // resourceVersion/uid/generation mirror what a real fetched TriggerRun's metadata carries;
    // the API rejects a create request that still has them set (see the sibling test below).
    const record = buildTerminalTriggerRun({
      metadata: {
        name: 'nightly',
        namespace: 'test-ns',
        resourceVersion: '42',
        uid: 'source-uid',
        generation: 3,
      },
    });
    const mockRequest = createQueryMockRouter({ CreateTriggerRun: { triggerRun: record } });

    render(
      <DetailViewHeader title="my-trigger" actions={TRIGGER_ACTIONS} record={record} />,
      buildWrapper(buildRerunWrappers(mockRequest))
    );

    await user.click(await screen.findByRole('button', { name: 'Rerun' }));
    const dialog = await screen.findByRole('dialog', { name: 'Rerun Trigger' });
    await user.click(within(dialog).getByRole('button', { name: 'Rerun' }));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith('CreateTriggerRun', expect.anything(), {});
    });

    const call = mockRequest.getCall('CreateTriggerRun');
    const payload = call?.args as TriggerRun;

    // A cron trigger with no batch/backfill markers is named with the "cron-" prefix,
    // followed by the source trigger's own name for traceability back to its definition.
    expect(payload.metadata.name).toMatch(/^cron-nightly-/);
    expect(payload.metadata.namespace).toBe('test-ns');
    // Pipeline, revision, and the trigger's own schedule carry over unchanged.
    expect(payload.spec.pipeline).toEqual({ name: 'my-pipeline', namespace: 'test-ns' });
    expect(payload.spec.revision).toEqual({ name: 'rev-1', namespace: 'test-ns' });
    expect(payload.spec.trigger).toEqual({
      triggerType: { case: 'cronSchedule', value: { cron: '0 * * * *' } },
    });
    // The new run must not spawn already killed, even though the source (being FAILED) is terminal.
    expect(payload.spec.action).toBe(TriggerRunAction.NO_ACTION);
    expect(payload.spec.kill).toBe(false);
    // Server-owned fields from the source run must not carry over onto the new one.
    expect(payload.spec.actor).toBeUndefined();
    expect(payload.status).toBeUndefined();
    // The API rejects a create request carrying a source object's resourceVersion/uid/
    // generation, so the new run's metadata must be rebuilt with only name and namespace.
    expect(payload.metadata).toEqual({ name: payload.metadata.name, namespace: 'test-ns' });
  });

  it('keeps the dialog open and shows the error when the mutation fails', async () => {
    const user = userEvent.setup();
    const record = buildTerminalTriggerRun();
    const mockRequest = createQueryMockRouter({ CreateTriggerRun: new Error('Rerun failed') });

    render(
      <DetailViewHeader title="my-trigger" actions={TRIGGER_ACTIONS} record={record} />,
      buildWrapper(buildRerunWrappers(mockRequest))
    );

    await user.click(await screen.findByRole('button', { name: 'Rerun' }));
    const dialog = await screen.findByRole('dialog', { name: 'Rerun Trigger' });
    await user.click(within(dialog).getByRole('button', { name: 'Rerun' }));

    await within(dialog).findByText(/Test error/);
    expect(screen.getByRole('dialog', { name: 'Rerun Trigger' })).toBeInTheDocument();
  });
});

describe('Trigger list page', () => {
  it('renders the column headers in order', async () => {
    render(
      <PhaseListRoute phases={{ train: TRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/triggers' }),
        getUserProviderWrapper(),
        getServiceProviderWrapper({
          request: vi.fn().mockResolvedValue({ triggerRunList: { items: [] } }),
        }),
      ])
    );

    const headers = await screen.findAllByRole('columnheader');
    const headerLabels = headers.map((header) => header.textContent).filter(Boolean);
    expect(headerLabels).toEqual([
      'Name',
      'Pipeline',
      'Creation time',
      'Cron',
      'Interval seconds',
      'Owner',
      'State',
      'Environment',
      'Auto switch to latest main',
    ]);
  });

  it('renders Cron/Interval, Environment, and Auto-switch values, with fallbacks', async () => {
    render(
      <PhaseListRoute phases={{ train: TRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/triggers' }),
        getUserProviderWrapper(),
        getServiceProviderWrapper({
          request: vi.fn().mockResolvedValue({
            triggerRunList: {
              items: [
                {
                  metadata: {
                    name: 'cron-trigger',
                    creationTimestamp: { seconds: 1660000000 },
                    labels: { 'michelangelo/environment': 'production' },
                  },
                  spec: {
                    pipeline: { name: 'my-pipeline' },
                    revision: { name: 'rev-1' },
                    trigger: {
                      triggerType: { case: 'cronSchedule', value: { cron: '0 2 * * *' } },
                    },
                    actor: { name: 'jsmith' },
                    autoFlip: true,
                  },
                  status: { state: 1 },
                },
                {
                  metadata: {
                    name: 'interval-trigger',
                    creationTimestamp: { seconds: 1650000000 },
                  },
                  spec: {
                    pipeline: { name: 'my-pipeline' },
                    revision: { name: 'rev-2' },
                    trigger: {
                      triggerType: {
                        case: 'intervalSchedule',
                        value: { interval: { seconds: 3600 } },
                      },
                    },
                    actor: { name: 'jsmith' },
                    autoFlip: false,
                  },
                  status: { state: 1 },
                },
              ],
            },
          }),
        }),
      ])
    );

    // Cron row: Cron cell shows the raw cron string, Interval seconds cell is blank (em dash).
    expect(await screen.findByText('0 2 * * *')).toBeInTheDocument();
    // Interval row: Interval seconds cell shows the numeric seconds value, Cron cell is blank.
    expect(await screen.findByText('3600')).toBeInTheDocument();
    // Both schedule-absent cells render the standard TextCell em-dash placeholder, one per row.
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(2);

    // Environment: present on the cron row, absent (blank) on the interval row.
    expect(screen.getByText('Production')).toBeInTheDocument();

    // Auto switch to latest main: true renders the label text via BooleanCell; false renders
    // nothing at all (no text node). Scope to each data row (index 0 is the header row) so the
    // assertion isn't satisfied by the column header repeating the same label text.
    const rows = await screen.findAllByRole('row');
    expect(within(rows[1]).getByText('Auto switch to latest main')).toBeInTheDocument();
    expect(within(rows[2]).queryByText('Auto switch to latest main')).not.toBeInTheDocument();
  });
});
