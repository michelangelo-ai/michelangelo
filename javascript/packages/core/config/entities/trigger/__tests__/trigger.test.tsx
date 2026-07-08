import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { InterpolatableActionsPopover } from '#core/components/actions/interpolatable-actions-popover';
import { TRIGGER_ENTITY_CONFIG } from '#core/config/entities/trigger/trigger';
import { TriggerRunAction, TriggerRunState } from '#core/config/entities/trigger/types';
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
import type { TriggerRun } from '#core/config/entities/trigger/types';

// PhaseEntityConfig.actions is ActionConfigSchema<T>[] where T is the entity's
// generic parameter; InterpolatableActionsPopover expects Data (Record<string, unknown>).
// TriggerRun is structurally compatible at runtime; cast to unify.
const KILL_ACTIONS = TRIGGER_ENTITY_CONFIG.actions as ActionConfigSchema<Data>[];

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
        })
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
