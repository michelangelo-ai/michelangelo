import { ARTWORK_TYPE } from 'baseui/banner';

import { Banner } from '#core/components/banner/banner';
import { ConfirmDialog } from '#core/components/modal/confirm-dialog/confirm-dialog';
import { Icon } from '#core/components/icon/icon';
import { useSchemaMiddleware } from '#core/hooks/use-schema-middleware/use-schema-middleware';
import { useStudioMutation } from '#core/hooks/use-studio-mutation';

import type { ActionConfig, ConfirmModalConfig, Data, MutationActionConfig } from './types';

type Props<T extends Data> = {
  action: ActionConfig<T> & { action: MutationActionConfig; modal: ConfirmModalConfig };
  record: T;
  onClose: () => void;
};

/**
 * Renders a {@link ConfirmDialog} for an action that fires a mutation. Applies
 * any declared middleware to the record before submission. The dialog
 * auto-closes on success and stays open with an error banner on failure.
 */
export function MutationConfirmDispatcher<T extends Data>({ action, record, onClose }: Props<T>) {
  const { applyMiddleware } = useSchemaMiddleware(action.action.middleware ?? null);
  const mutation = useStudioMutation<unknown, T>(action.action.mutation);

  return (
    <ConfirmDialog
      isOpen
      onDismiss={onClose}
      onConfirm={async () => {
        await mutation.mutateAsync(applyMiddleware(record));
      }}
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
