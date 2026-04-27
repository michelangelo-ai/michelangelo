import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { ActionMenu } from '#core/components/actions/action-menu/action-menu';
import { ActionsPopover } from '#core/components/actions/actions-popover';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

import type { ActionComponentProps } from '#core/components/actions/types';

describe('ActionsPopover', () => {
  function DeleteDialog({ isOpen }: ActionComponentProps) {
    return isOpen ? <div role="dialog">Delete dialog</div> : null;
  }

  it('renders an "Actions" trigger button', () => {
    render(
      <ActionsPopover
        actions={[{ display: { label: 'Delete' }, component: DeleteDialog }]}
        record={{}}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );
    expect(screen.getByRole('button', { name: 'Actions' })).toBeInTheDocument();
  });

  it('does not show menu items before the trigger is clicked', () => {
    render(
      <ActionsPopover
        actions={[{ display: { label: 'Delete' }, component: DeleteDialog }]}
        record={{}}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );
    expect(screen.queryByRole('option', { name: 'Delete' })).not.toBeInTheDocument();
  });

  it('shows menu items when the trigger is clicked', async () => {
    const user = userEvent.setup();
    render(
      <ActionsPopover
        actions={[{ display: { label: 'Delete' }, component: DeleteDialog }]}
        record={{}}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );
    await user.click(screen.getByRole('button', { name: 'Actions' }));
    expect(await screen.findByRole('option', { name: 'Delete' })).toBeInTheDocument();
  });

  it('renders an action menu item with an icon', async () => {
    const user = userEvent.setup();
    render(
      <ActionsPopover
        actions={[{ display: { label: 'Delete', icon: 'trash' }, component: DeleteDialog }]}
        record={{}}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper({ icons: { trash: () => <div>Trash</div> } }),
      ])
    );

    await user.click(screen.getByRole('button', { name: 'Actions' }));
    expect(await screen.findByRole('option', { name: /Trash Delete/ })).toBeInTheDocument();
  });

  it('opens the action component and closes the menu when a menu item is clicked', async () => {
    const user = userEvent.setup();
    render(
      <ActionsPopover
        actions={[{ display: { label: 'Delete' }, component: DeleteDialog }]}
        record={{}}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
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
    const Component = ({ record, isOpen }: ActionComponentProps) =>
      isOpen ? <div role="dialog">{String(record.id)}</div> : null;
    const data = { id: '42', type: 'pipeline' };
    render(
      <ActionsPopover
        actions={[{ display: { label: 'Run' }, component: Component }]}
        record={data}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );
    await user.click(screen.getByRole('button', { name: 'Actions' }));
    await user.click(await screen.findByRole('option', { name: 'Run' }));
    expect(await screen.findByRole('dialog')).toHaveTextContent('42');
  });

  it('disables body scroll when opened and restores it on unmount', async () => {
    const user = userEvent.setup();
    const { unmount } = render(
      <ActionsPopover
        actions={[{ display: { label: 'Delete' }, component: DeleteDialog }]}
        record={{}}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
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
        actions={[{ display: { label: 'Delete' }, component: DeleteDialog }]}
        record={{}}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
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
      component: DeleteDialog,
      disabled: [{ condition: true, message: 'Cannot delete' }],
    };

    it('renders a disabled action as visible in the menu', async () => {
      const user = userEvent.setup();
      render(
        <ActionsPopover actions={[disabledAction]} record={{}} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );
      await user.click(screen.getByRole('button', { name: 'Actions' }));
      expect(await screen.findByRole('option', { name: 'Delete' })).toBeInTheDocument();
    });

    it('does not open the action component when a disabled action is clicked', async () => {
      const user = userEvent.setup();
      render(
        <ActionsPopover actions={[disabledAction]} record={{}} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );
      await user.click(screen.getByRole('button', { name: 'Actions' }));
      await user.click(await screen.findByRole('option', { name: 'Delete' }));
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });

    it('shows the disabled message tooltip when the item is hovered', async () => {
      const user = userEvent.setup();
      render(
        <ActionsPopover actions={[disabledAction]} record={{}} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );
      await user.click(screen.getByRole('button', { name: 'Actions' }));
      await user.hover(await screen.findByRole('option', { name: 'Delete' }));
      expect(await screen.findByText('Cannot delete')).toBeInTheDocument();
    });

    it('closes the menu on Escape when the disabled tooltip is visible', async () => {
      const user = userEvent.setup();
      render(
        <ActionsPopover actions={[disabledAction]} record={{}} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
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
              component: DeleteDialog,
              disabled: [{ condition: true, message: 'Item is locked' }],
            },
          ]}
          record={{ status: 'locked' }}
        />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
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
              component: DeleteDialog,
              disabled: [
                { condition: false, message: 'Should not appear' },
                { condition: true, message: 'Second rule matches' },
              ],
            },
          ]}
          record={{}}
        />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );
      await user.click(screen.getByRole('button', { name: 'Actions' }));
      await user.hover(await screen.findByRole('option', { name: 'Delete' }));
      expect(await screen.findByText('Second rule matches')).toBeInTheDocument();
      expect(screen.queryByText('Should not appear')).not.toBeInTheDocument();
    });

    it('shows only one tooltip at a time when hovering between two disabled items', async () => {
      const user = userEvent.setup();
      render(
        <ActionMenu
          actions={[
            {
              display: { label: 'Delete' },
              component: DeleteDialog,
              disabled: [{ condition: true, message: 'Cannot delete' }],
            },
            {
              display: { label: 'Archive' },
              component: DeleteDialog,
              disabled: [{ condition: true, message: 'Cannot archive' }],
            },
          ]}
          record={{}}
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
        <ActionMenu actions={[disabledAction]} record={{}} onSelectAction={vi.fn()} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );
      await screen.findByRole('option', { name: 'Delete' });
      expect(screen.queryByText('Cannot delete')).not.toBeInTheDocument();
    });

    it('shows the tooltip when a disabled item is highlighted via keyboard', async () => {
      const user = userEvent.setup();
      render(
        <ActionMenu
          actions={[{ display: { label: 'Edit' }, component: DeleteDialog }, disabledAction]}
          record={{}}
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
              component: DeleteDialog,
              disabled: [{ condition: true, message: 'Cannot delete' }],
            },
            {
              display: { label: 'Archive' },
              component: DeleteDialog,
              disabled: [{ condition: true, message: 'Cannot archive' }],
            },
          ]}
          record={{}}
          onSelectAction={vi.fn()}
        />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );
      await user.tab();
      await user.keyboard('{ArrowDown}'); // highlights Archive (index 1)
      expect(await screen.findByText('Cannot archive')).toBeInTheDocument();
      await user.hover(screen.getByRole('option', { name: 'Delete' }));
      expect(await screen.findByText('Cannot delete')).toBeInTheDocument();
      expect(screen.queryByText('Cannot archive')).not.toBeInTheDocument();
    });

    it('enabled and disabled actions coexist — enabled action still opens its component', async () => {
      const user = userEvent.setup();
      render(
        <ActionsPopover
          actions={[disabledAction, { display: { label: 'Edit' }, component: DeleteDialog }]}
          record={{}}
        />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );
      await user.click(screen.getByRole('button', { name: 'Actions' }));
      await user.click(await screen.findByRole('option', { name: 'Edit' }));
      expect(await screen.findByRole('dialog')).toBeInTheDocument();
    });
  });
});
