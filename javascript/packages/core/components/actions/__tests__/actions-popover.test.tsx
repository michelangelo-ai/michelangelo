import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

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
      disabled: [{ condition: () => true, message: 'Cannot delete' }],
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

    it('passes the record to the disabled condition', async () => {
      const user = userEvent.setup();
      const condition = vi.fn().mockReturnValue(false);
      const record = { id: '42' };
      render(
        <ActionsPopover
          actions={[
            {
              display: { label: 'Delete' },
              component: DeleteDialog,
              disabled: [{ condition, message: 'nope' }],
            },
          ]}
          record={record}
        />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );
      await user.click(screen.getByRole('button', { name: 'Actions' }));
      await screen.findByRole('option', { name: 'Delete' });
      expect(condition).toHaveBeenCalledWith(record);
    });
  });
});
