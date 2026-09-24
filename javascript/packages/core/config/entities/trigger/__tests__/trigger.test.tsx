import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { InterpolatableActionsPopover } from '#core/components/actions/interpolatable-actions-popover';
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
// generic parameter; InterpolatableActionsPopover/DetailViewHeader expect Data
// (Record<string, unknown>). TriggerRun is structurally compatible at runtime; cast to unify.
const TRIGGER_ACTIONS = TRIGGER_ENTITY_CONFIG.actions as ActionConfigSchema<Data>[];
// Kept as an alias for the existing Kill-focused tests below, which predate Rerun.
const KILL_ACTIONS = TRIGGER_ACTIONS;

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
  it('opens a confirm dialog naming the run and pipeline, fires UpdateTriggerRun with spec.action=KILL', async () => {
    const user = userEvent.setup();
    const record = buildRunningTriggerRun();
    const mockRequest = createQueryMockRouter({ UpdateTriggerRun: { triggerRun: record } });

    render(
      <InterpolatableActionsPopover actions={KILL_ACTIONS} record={record} />,
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

    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Kill' }));

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

  it('disables the action with a tooltip when the run is not killable', async () => {
    const user = userEvent.setup();
    const record = buildRunningTriggerRun({ status: { state: TriggerRunState.SUCCEEDED } });

    render(
      <InterpolatableActionsPopover actions={KILL_ACTIONS} record={record} />,
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

    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.hover(await screen.findByRole('option', { name: 'Kill' }));
    expect(
      await screen.findByText('Only running or paused trigger runs can be killed')
    ).toBeInTheDocument();
  });

  it('keeps dialog open and shows the error when the mutation fails', async () => {
    const user = userEvent.setup();
    const record = buildRunningTriggerRun();
    const mockRequest = createQueryMockRouter({ UpdateTriggerRun: new Error('Kill failed') });

    render(
      <InterpolatableActionsPopover actions={KILL_ACTIONS} record={record} />,
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

    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Kill' }));
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

  it('renders Rerun as a direct header button even when Kill is demoted to the overflow menu', async () => {
    const record = buildTerminalTriggerRun();

    render(
      <DetailViewHeader title="my-trigger" actions={TRIGGER_ACTIONS} record={record} />,
      buildWrapper(buildRerunWrappers(vi.fn()))
    );

    // A failed run isn't killable, so Kill collapses into the overflow menu — but Rerun is
    // a static secondary action and always renders directly as its own button.
    expect(await screen.findByRole('button', { name: 'Rerun' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Actions' })).toBeInTheDocument();
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
    const record = buildTerminalTriggerRun();
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

    // A cron trigger with no batch/backfill markers is named with the "cron-" prefix.
    expect(payload.metadata.name).toMatch(/^cron-/);
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
