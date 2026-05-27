import { useNavigate } from 'react-router-dom-v5-compat';
import { ARTWORK_TYPE } from 'baseui/banner';

import { Banner } from '#core/components/banner/banner';
import { ConfirmDialog } from '#core/components/modal/confirm-dialog/confirm-dialog';
import { Icon } from '#core/components/icon/icon';

import type { ActionConfig, ConfirmModalConfig, Data, RouteActionConfig } from './types';

type Props<T extends Data> = {
  action: ActionConfig<T> & { action: RouteActionConfig; modal: ConfirmModalConfig };
  onClose: () => void;
};

/**
 * Renders a {@link ConfirmDialog} for an action that navigates to a route.
 */
export function RouteConfirmDispatcher<T extends Data>({ action, onClose }: Props<T>) {
  const navigate = useNavigate();

  return (
    <ConfirmDialog
      isOpen
      onDismiss={onClose}
      onConfirm={() => navigate(action.action.route)}
      heading={action.modal.header.title}
      confirmLabel={action.modal.button.label}
      destructive={action.modal.destructive}
      size={action.modal.size}
    >
      {action.modal.banner && (
        <Banner
          kind={action.modal.banner.kind}
          artwork={
            action.modal.banner.icon
              ? { type: ARTWORK_TYPE.icon, icon: () => <Icon name={action.modal.banner!.icon!} /> }
              : undefined
          }
        >
          {action.modal.banner.content}
        </Banner>
      )}
      {action.modal.body}
    </ConfirmDialog>
  );
}
