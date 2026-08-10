import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

// NOTE: these test utilities and this component are internal to packages/core and are not
// part of its public export surface, so this test — which moved to app/config with the rest
// of the trigger entity config — reaches into core's source tree via a relative path rather
// than through the package's public API. See PR 2 of the config extraction plan for context.
import { InterpolatableActionsPopover } from '../../../../../packages/core/components/actions/interpolatable-actions-popover';
import { buildWrapper } from '../../../../../packages/core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '../../../../../packages/core/test/wrappers/get-base-provider-wrapper';
import { getErrorProviderWrapper } from '../../../../../packages/core/test/wrappers/get-error-provider-wrapper';
import { getIconProviderWrapper } from '../../../../../packages/core/test/wrappers/get-icon-provider-wrapper';
import { getInterpolationProviderWrapper } from '../../../../../packages/core/test/wrappers/get-interpolation-provider-wrapper';
import { getRouterWrapper } from '../../../../../packages/core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '../../../../../packages/core/test/wrappers/get-service-provider-wrapper';
import { getSnackbarProviderWrapper } from '../../../../../packages/core/test/wrappers/get-snackbar-provider-wrapper';
import { TRIGGER_ENTITY_CONFIG } from '../trigger';
import { TriggerRunAction, TriggerRunState } from '../types';

import type { ActionConfigSchema, Data } from '@michelangelo-ai/core';
import type { TriggerRun } from '../types';

// cast: PhaseEntityConfig.actions is ActionConfigSchema<T>[] where T is the entity's generic
// parameter; InterpolatableActionsPopover expects Data (Record<string, unknown>). TriggerRun
// is structurally compatible at runtime; see #1425
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
          // cast: expect.objectContaining returns a matcher typed as the argument shape, not
          // Record<string, unknown>; asserting the mock-call shape used elsewhere in this test
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
