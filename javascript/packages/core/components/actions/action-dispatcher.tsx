import { ConfirmDispatcher } from './confirm-dispatcher';

import type { ActionConfig, Data } from './types';

type Props<T extends Data> = {
  action: ActionConfig<T>;
  record: T;
  onClose: () => void;
};

export function ActionDispatcher<T extends Data>({ action, record, onClose }: Props<T>) {
  if (action.modal?.type === 'custom') {
    const Component = action.modal.component;
    return <Component record={record} onClose={onClose} />;
  }
  if (action.modal?.type === 'confirm' && action.action) {
    return (
      <ConfirmDispatcher
        action={{ ...action, action: action.action, modal: action.modal }}
        record={record}
        onClose={onClose}
      />
    );
  }
  return null;
}
