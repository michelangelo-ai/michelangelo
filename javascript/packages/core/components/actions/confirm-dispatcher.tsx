import { useNavigate } from 'react-router-dom-v5-compat';
import { ARTWORK_TYPE } from 'baseui/banner';

import { Banner } from '#core/components/banner/banner';
import { Icon } from '#core/components/icon/icon';
import { Markdown } from '#core/components/markdown/markdown';
import { ConfirmDialog } from '#core/components/modal/confirm-dialog/confirm-dialog';
import { useSchemaMiddleware } from '#core/hooks/use-schema-middleware/use-schema-middleware';
import { useStudioMutation } from '#core/hooks/use-studio-mutation';
import { useSuccessOperations } from './use-success-operations';

import type {
  ActionConfig,
  ConfirmModalConfig,
  Data,
  MutationActionConfig,
  RouteActionConfig,
} from './types';

type Props<T extends Data> = {
  action: ActionConfig<T> & {
    operation: MutationActionConfig | RouteActionConfig;
    modal: ConfirmModalConfig;
  };
  record: T;
  onClose: () => void;
};

export function ConfirmDispatcher<T extends Data>({ action, record, onClose }: Props<T>) {
  const navigate = useNavigate();
  const { applyMiddleware } = useSchemaMiddleware(
    action.operation.type === 'mutation' ? (action.operation.middleware ?? null) : null
  );
  const mutation = useStudioMutation<unknown, T>(
    action.operation.type === 'mutation' ? action.operation.mutation : null
  );
  const runSuccessOperations = useSuccessOperations(
    action.operation.type === 'mutation' ? action.operation.successOperations : undefined
  );

  const executeAction = async () => {
    if (action.operation.type === 'mutation') {
      const response = await mutation.mutateAsync(applyMiddleware(record));
      runSuccessOperations(response);
    } else {
      navigate(action.operation.route);
    }
  };

  const { modal } = action;
  return (
    <ConfirmDialog
      isOpen
      onDismiss={onClose}
      onConfirm={executeAction}
      heading={modal.header.title}
      confirmLabel={modal.button.label}
      destructive={modal.destructive}
      size={modal.size}
    >
      {modal.banner && (
        <Banner
          kind={modal.banner.kind}
          artwork={
            modal.banner.icon
              ? { type: ARTWORK_TYPE.icon, icon: () => <Icon name={modal.banner!.icon} /> }
              : undefined
          }
        >
          {modal.banner.content}
        </Banner>
      )}
      {modal.body && <Markdown>{modal.body}</Markdown>}
    </ConfirmDialog>
  );
}
