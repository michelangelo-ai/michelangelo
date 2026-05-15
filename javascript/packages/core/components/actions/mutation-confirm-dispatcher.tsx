import { Banner } from '#core/components/banner/banner';
import { ConfirmDialog } from '#core/components/modal/confirm-dialog/confirm-dialog';
import { useSchemaMiddleware } from '#core/hooks/use-schema-middleware/use-schema-middleware';
import { useStudioMutation } from '#core/hooks/use-studio-mutation';
import { useSuccessOperations } from './use-success-operations';

import type { ActionConfig, ConfirmModalConfig, Data, MutationActionConfig } from './types';

type Props<T extends Data> = {
  action: ActionConfig<T> & { operation: MutationActionConfig; modal: ConfirmModalConfig };
  record: T;
  onClose: () => void;
};

/**
 * Renders a {@link ConfirmDialog} for an action that fires a mutation. Applies
 * any declared middleware to the record before submission. The dialog
 * auto-closes on success and stays open with an error banner on failure.
 *
 * On success, runs any declared `successOperations` (toast, additional
 * invalidations) against the mutation response.
 */
export function MutationConfirmDispatcher<T extends Data>({ action, record, onClose }: Props<T>) {
  const { applyMiddleware } = useSchemaMiddleware(action.operation.middleware ?? null);
  const mutation = useStudioMutation<unknown, T>(action.operation.mutation);
  const runSuccessOperations = useSuccessOperations(action.operation.successOperations);

  return (
    <ConfirmDialog
      isOpen
      onDismiss={onClose}
      onConfirm={async () => {
        const response = await mutation.mutateAsync(applyMiddleware(record));
        runSuccessOperations(response);
      }}
      heading={action.modal.header.title}
      confirmLabel={action.modal.button.label}
      destructive={action.modal.destructive}
      size={action.modal.size}
    >
      {action.modal.banner && (
        <Banner kind={action.modal.banner.kind}>{action.modal.banner.content}</Banner>
      )}
      {action.modal.body}
    </ConfirmDialog>
  );
}
