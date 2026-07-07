import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { ActionMenu } from '#core/components/actions/action-menu/action-menu';
import { ActionsPopover } from '#core/components/actions/actions-popover';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';
import { getSnackbarProviderWrapper } from '#core/test/wrappers/get-snackbar-provider-wrapper';

import type { ActionComponentProps } from '#core/components/actions/types';

describe('ActionsPopover', () => {
  function DeleteDialog({ record }: ActionComponentProps) {
    const id = typeof record.id === 'string' ? record.id : '';
    return <div role="dialog">Delete dialog {id}</div>;
  }

  it('renders an "Actions" trigger button', () => {
    render(
      <ActionsPopover
        actions={[
          { display: { label: 'Delete' }, modal: { type: 'custom', component: DeleteDialog } },
        ]}
        record={{}}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getRouterWrapper()])
    );
    expect(screen.getByRole('button', { name: 'Actions' })).toBeInTheDocument();
  });

  it('does not show menu items before the trigger is clicked', () => {
    render(
      <ActionsPopover
        actions={[
          { display: { label: 'Delete' }, modal: { type: 'custom', component: DeleteDialog } },
        ]}
        record={{}}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getRouterWrapper()])
    );
    expect(screen.queryByRole('option', { name: 'Delete' })).not.toBeInTheDocument();
  });

  it('shows menu items when the trigger is clicked', async () => {
    const user = userEvent.setup();
    render(
      <ActionsPopover
        actions={[
          { display: { label: 'Delete' }, modal: { type: 'custom', component: DeleteDialog } },
        ]}
        record={{}}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getRouterWrapper()])
    );
    await user.click(screen.getByRole('button', { name: 'Actions' }));
    expect(await screen.findByRole('option', { name: 'Delete' })).toBeInTheDocument();
  });

  it('renders an action menu item with an icon', async () => {
    const user = userEvent.setup();
    render(
      <ActionsPopover
        actions={[
          {
            display: { label: 'Delete', icon: 'trash' },
            modal: { type: 'custom', component: DeleteDialog },
          },
        ]}
        record={{}}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper({ icons: { trash: () => <div>Trash</div> } }),
        getRouterWrapper(),
      ])
    );

    await user.click(screen.getByRole('button', { name: 'Actions' }));
    expect(await screen.findByRole('option', { name: /Trash Delete/ })).toBeInTheDocument();
  });

  it('opens the action component and closes the menu when a menu item is clicked', async () => {
    const user = userEvent.setup();
    render(
      <ActionsPopover
        actions={[
          { display: { label: 'Delete' }, modal: { type: 'custom', component: DeleteDialog } },
        ]}
        record={{}}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getRouterWrapper()])
    );
    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Delete' }));
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByRole('option', { name: 'Delete' })).not.toBeInTheDocument();
    });
  });

  it('passes data to the action component', async () => {
    const user = userEvent.setup();
    const Component = ({ record }: ActionComponentProps) => (
      <div role="dialog">{String(record.id)}</div>
    );
    const data = { id: '42', type: 'pipeline' };
    render(
      <ActionsPopover
        actions={[{ display: { label: 'Run' }, modal: { type: 'custom', component: Component } }]}
        record={data}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getRouterWrapper()])
    );
    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Run' }));
    expect(await screen.findByRole('dialog')).toHaveTextContent('42');
  });

  it('disables body scroll when opened and restores it on unmount', async () => {
    const user = userEvent.setup();
    const { unmount } = render(
      <ActionsPopover
        actions={[
          { display: { label: 'Delete' }, modal: { type: 'custom', component: DeleteDialog } },
        ]}
        record={{}}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getRouterWrapper()])
    );
    await user.click(screen.getByRole('button', { name: 'Actions' }));
    expect(document.body.style.overflow).toBe('hidden');
    unmount();
    expect(document.body.style.overflow).toBe('');
  });

  it('closes menu on Escape', async () => {
    const user = userEvent.setup();
    render(
      <ActionsPopover
        actions={[
          { display: { label: 'Delete' }, modal: { type: 'custom', component: DeleteDialog } },
        ]}
        record={{}}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getRouterWrapper()])
    );
    await user.click(screen.getByRole('button', { name: 'Actions' }));
    expect(await screen.findByRole('option', { name: 'Delete' })).toBeInTheDocument();
    await user.keyboard('{Escape}');
    await waitFor(() => {
      expect(screen.queryByRole('option', { name: 'Delete' })).not.toBeInTheDocument();
    });
  });

  describe('disabled actions', () => {
    const disabledAction = {
      display: { label: 'Delete' },
      modal: { type: 'custom' as const, component: DeleteDialog },
      disabled: [{ condition: true, message: 'Cannot delete' }],
    };

    it('renders a disabled action as visible in the menu', async () => {
      const user = userEvent.setup();
      render(
        <ActionsPopover actions={[disabledAction]} record={{}} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getRouterWrapper()])
      );
      await user.click(screen.getByRole('button', { name: 'Actions' }));
      expect(await screen.findByRole('option', { name: 'Delete' })).toBeInTheDocument();
    });

    it('does not open the action component when a disabled action is clicked', async () => {
      const user = userEvent.setup();
      render(
        <ActionsPopover actions={[disabledAction]} record={{}} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getRouterWrapper()])
      );
      await user.click(screen.getByRole('button', { name: 'Actions' }));
      await user.click(await screen.findByRole('option', { name: 'Delete' }));
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });

    it('shows the disabled message tooltip when the item is hovered', async () => {
      const user = userEvent.setup();
      render(
        <ActionsPopover actions={[disabledAction]} record={{}} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getRouterWrapper()])
      );
      await user.click(screen.getByRole('button', { name: 'Actions' }));
      await user.hover(await screen.findByRole('option', { name: 'Delete' }));
      expect(await screen.findByText('Cannot delete')).toBeInTheDocument();
    });

    it('closes the menu on Escape when the disabled tooltip is visible', async () => {
      const user = userEvent.setup();
      render(
        <ActionsPopover actions={[disabledAction]} record={{}} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getRouterWrapper()])
      );
      await user.click(screen.getByRole('button', { name: 'Actions' }));
      await user.hover(await screen.findByRole('option', { name: 'Delete' }));
      await screen.findByText('Cannot delete'); // tooltip visible confirms item is highlighted
      await user.keyboard('{Escape}');
      await waitFor(() => {
        expect(screen.queryByRole('option', { name: 'Delete' })).not.toBeInTheDocument();
      });
    });

    it('shows the disabled message when condition is true', async () => {
      const user = userEvent.setup();
      render(
        <ActionsPopover
          actions={[
            {
              display: { label: 'Delete' },
              modal: { type: 'custom', component: DeleteDialog },
              disabled: [{ condition: true, message: 'Item is locked' }],
            },
          ]}
          record={{ status: 'locked' }}
        />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getRouterWrapper()])
      );
      await user.click(screen.getByRole('button', { name: 'Actions' }));
      await user.hover(await screen.findByRole('option', { name: 'Delete' }));
      expect(await screen.findByText('Item is locked')).toBeInTheDocument();
    });

    it('uses the first matching rule message', async () => {
      const user = userEvent.setup();
      render(
        <ActionsPopover
          actions={[
            {
              display: { label: 'Delete' },
              modal: { type: 'custom', component: DeleteDialog },
              disabled: [
                { condition: false, message: 'Should not appear' },
                { condition: true, message: 'Second rule matches' },
              ],
            },
          ]}
          record={{}}
        />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getRouterWrapper()])
      );
      await user.click(screen.getByRole('button', { name: 'Actions' }));
      await user.hover(await screen.findByRole('option', { name: 'Delete' }));
      expect(await screen.findByText('Second rule matches')).toBeInTheDocument();
      expect(screen.queryByText('Should not appear')).not.toBeInTheDocument();
    });

    it('ignores subsequent matching rules after the first', async () => {
      const user = userEvent.setup();
      render(
        <ActionsPopover
          actions={[
            {
              display: { label: 'Delete' },
              modal: { type: 'custom', component: DeleteDialog },
              disabled: [
                { condition: false, message: 'Should not appear' },
                { condition: true, message: 'Second rule matches' },
                { condition: true, message: 'Third rule should not appear' },
              ],
            },
          ]}
          record={{}}
        />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getRouterWrapper()])
      );
      await user.click(screen.getByRole('button', { name: 'Actions' }));
      await user.hover(await screen.findByRole('option', { name: 'Delete' }));
      expect(await screen.findByText('Second rule matches')).toBeInTheDocument();
      expect(screen.queryByText('Should not appear')).not.toBeInTheDocument();
      expect(screen.queryByText('Third rule should not appear')).not.toBeInTheDocument();
    });

    it('shows only one tooltip at a time when hovering between two disabled items', async () => {
      const user = userEvent.setup();
      render(
        <ActionMenu
          actions={[
            {
              display: { label: 'Delete' },
              disabled: true,
              disabledMessage: 'Cannot delete',
              onClick: vi.fn(),
            },
            {
              display: { label: 'Archive' },
              disabled: true,
              disabledMessage: 'Cannot archive',
              onClick: vi.fn(),
            },
          ]}
          onSelectAction={vi.fn()}
        />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );
      const deleteOption = await screen.findByRole('option', { name: 'Delete' });
      const archiveOption = screen.getByRole('option', { name: 'Archive' });
      await user.hover(deleteOption);
      await screen.findByText('Cannot delete');
      await user.hover(archiveOption);
      expect(await screen.findByText('Cannot archive')).toBeInTheDocument();
      await waitFor(() => {
        expect(screen.queryByText('Cannot delete')).not.toBeInTheDocument();
      });
    });

    it('does not show the tooltip from auto-highlight when the menu opens', async () => {
      render(
        <ActionMenu
          actions={[
            {
              display: { label: 'Delete' },
              disabled: true,
              disabledMessage: 'Cannot delete',
              onClick: vi.fn(),
            },
          ]}
          onSelectAction={vi.fn()}
        />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );
      await screen.findByRole('option', { name: 'Delete' });
      expect(screen.queryByText('Cannot delete')).not.toBeInTheDocument();
    });

    it('shows the tooltip when a disabled item is highlighted via keyboard', async () => {
      const user = userEvent.setup();
      render(
        <ActionMenu
          actions={[
            { display: { label: 'Edit' }, disabled: false, onClick: vi.fn() },
            {
              display: { label: 'Delete' },
              disabled: true,
              disabledMessage: 'Cannot delete',
              onClick: vi.fn(),
            },
          ]}
          onSelectAction={vi.fn()}
        />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );
      await user.tab(); // focus the menu listbox
      await user.keyboard('{ArrowDown}');
      expect(await screen.findByText('Cannot delete')).toBeInTheDocument();
    });

    it('switches from keyboard tooltip to mouse tooltip when hovering a different item', async () => {
      const user = userEvent.setup();
      render(
        <ActionMenu
          actions={[
            {
              display: { label: 'Delete' },
              disabled: true,
              disabledMessage: 'Cannot delete',
              onClick: vi.fn(),
            },
            {
              display: { label: 'Archive' },
              disabled: true,
              disabledMessage: 'Cannot archive',
              onClick: vi.fn(),
            },
          ]}
          onSelectAction={vi.fn()}
        />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );
      await user.tab();
      await user.keyboard('{ArrowDown}'); // highlights Archive (index 1)
      expect(await screen.findByText('Cannot archive')).toBeInTheDocument();
      await user.hover(screen.getByRole('option', { name: 'Delete' }));
      expect(await screen.findByText('Cannot delete')).toBeInTheDocument();
      await waitFor(() => {
        expect(screen.queryByText('Cannot archive')).not.toBeInTheDocument();
      });
    });

    it('enabled and disabled actions coexist — enabled action still opens its component', async () => {
      const user = userEvent.setup();
      render(
        <ActionsPopover
          actions={[
            disabledAction,
            { display: { label: 'Edit' }, modal: { type: 'custom', component: DeleteDialog } },
          ]}
          record={{}}
        />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getRouterWrapper()])
      );
      await user.click(screen.getByRole('button', { name: 'Actions' }));
      await user.click(await screen.findByRole('option', { name: 'Edit' }));
      expect(await screen.findByRole('dialog')).toBeInTheDocument();
    });
  });

  it('mutation-confirm action: shows confirm dialog and fires mutation on confirm', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({ UpdateTriggerRun: { triggerRun: {} } });

    render(
      <ActionsPopover
        actions={[
          {
            display: { label: 'Kill' },
            operation: { type: 'mutation', mutation: { mutationName: 'UpdateTriggerRun' } },
            modal: {
              type: 'confirm',
              header: { title: 'Confirm kill?' },
              button: { label: 'Kill it' },
              destructive: true,
            },
          },
        ]}
        record={{ id: 'run-1' }}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getRouterWrapper(),
        getServiceProviderWrapper({ request: mockRequest }),
        getSnackbarProviderWrapper(),
      ])
    );

    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Kill' }));
    const dialog = await screen.findByRole('dialog', { name: 'Confirm kill?' });
    await user.click(within(dialog).getByRole('button', { name: 'Kill it' }));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith('UpdateTriggerRun', { id: 'run-1' });
    });
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
  });

  it('mutation-confirm action: keeps dialog open and shows error on mutation failure', async () => {
    const user = userEvent.setup();
    const failingRequest = vi.fn().mockRejectedValue(new Error('rpc error'));

    render(
      <ActionsPopover
        actions={[
          {
            display: { label: 'Kill' },
            operation: { type: 'mutation', mutation: { mutationName: 'UpdateTriggerRun' } },
            modal: {
              type: 'confirm',
              header: { title: 'Confirm kill?' },
              button: { label: 'Kill it' },
              destructive: true,
            },
          },
        ]}
        record={{ id: 'run-1' }}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getRouterWrapper(),
        getServiceProviderWrapper({ request: failingRequest }),
        getSnackbarProviderWrapper(),
      ])
    );

    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Kill' }));
    const dialog = await screen.findByRole('dialog', { name: 'Confirm kill?' });
    await user.click(within(dialog).getByRole('button', { name: 'Kill it' }));

    expect(await screen.findByRole('dialog', { name: 'Confirm kill?' })).toBeInTheDocument();
    expect(await screen.findByText('Test error')).toBeInTheDocument();
  });

  it('route-confirm action: shows confirm dialog and navigates on confirm', async () => {
    const user = userEvent.setup();

    render(
      <ActionsPopover
        actions={[
          {
            display: { label: 'Open detail' },
            operation: { type: 'route', route: '/dest/page' },
            modal: {
              type: 'confirm',
              header: { title: 'Open detail page?' },
              button: { label: 'Open' },
            },
          },
        ]}
        record={{}}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/start' }),
        getServiceProviderWrapper({ request: vi.fn() }),
        getSnackbarProviderWrapper(),
      ])
    );

    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Open detail' }));
    const dialog = await screen.findByRole('dialog', { name: 'Open detail page?' });
    await user.click(within(dialog).getByRole('button', { name: 'Open' }));

    expect(screen.getByText(/Current pathname: \/dest\/page/)).toBeInTheDocument();
  });

  it('mutation-confirm action: cancel closes dialog without mutating', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({ UpdateTriggerRun: {} });

    render(
      <ActionsPopover
        actions={[
          {
            display: { label: 'Kill' },
            operation: { type: 'mutation', mutation: { mutationName: 'UpdateTriggerRun' } },
            modal: {
              type: 'confirm',
              header: { title: 'Confirm kill?' },
              button: { label: 'Kill it' },
              destructive: true,
            },
          },
        ]}
        record={{ id: 'run-1' }}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getRouterWrapper(),
        getServiceProviderWrapper({ request: mockRequest }),
        getSnackbarProviderWrapper(),
      ])
    );

    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Kill' }));
    const dialog = await screen.findByRole('dialog', { name: 'Confirm kill?' });
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    expect(mockRequest).not.toHaveBeenCalled();
  });

  it('mutation-confirm action: shows success toast after mutation', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({ UpdateTriggerRun: {} });

    render(
      <ActionsPopover
        actions={[
          {
            display: { label: 'Kill' },
            operation: {
              type: 'mutation',
              mutation: {
                mutationName: 'UpdateTriggerRun',
                successOperations: [{ type: 'toast', message: 'Trigger killed' }],
              },
            },
            modal: {
              type: 'confirm',
              header: { title: 'Confirm kill?' },
              button: { label: 'Kill it' },
            },
          },
        ]}
        record={{ id: 'run-1' }}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getRouterWrapper(),
        getServiceProviderWrapper({ request: mockRequest }),
        getSnackbarProviderWrapper(),
      ])
    );

    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Kill' }));
    await user.click(
      within(await screen.findByRole('dialog', { name: 'Confirm kill?' })).getByRole('button', {
        name: 'Kill it',
      })
    );

    expect(await screen.findByText('Trigger killed')).toBeInTheDocument();
  });

  it('mutation-confirm action: does not show success toast on failure', async () => {
    const user = userEvent.setup();
    const failingRequest = vi.fn().mockRejectedValue(new Error('rpc error'));

    render(
      <ActionsPopover
        actions={[
          {
            display: { label: 'Kill' },
            operation: {
              type: 'mutation',
              mutation: {
                mutationName: 'UpdateTriggerRun',
                successOperations: [{ type: 'toast', message: 'Should not appear' }],
              },
            },
            modal: {
              type: 'confirm',
              header: { title: 'Confirm kill?' },
              button: { label: 'Kill it' },
            },
          },
        ]}
        record={{ id: 'run-1' }}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getErrorProviderWrapper(),
        getRouterWrapper(),
        getServiceProviderWrapper({ request: failingRequest }),
        getSnackbarProviderWrapper(),
      ])
    );

    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Kill' }));
    const dialog = await screen.findByRole('dialog', { name: 'Confirm kill?' });
    await user.click(within(dialog).getByRole('button', { name: 'Kill it' }));

    await within(dialog).findByText(/Test error/);
    expect(screen.queryByText('Should not appear')).not.toBeInTheDocument();
  });
});
