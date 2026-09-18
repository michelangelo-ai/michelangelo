import { ConfirmDispatcher } from './confirm-dispatcher';

import type {
  ActionConfig,
  ConfirmModalConfig,
  Data,
  MutationActionConfig,
  RevisionRef,
  RouteActionConfig,
} from './types';

type Props<T extends Data> = {
  action: ActionConfig<T>;
  record: T;
  revision?: RevisionRef;
  onClose: () => void;
};

export function ActionDispatcher<T extends Data>({ action, record, revision, onClose }: Props<T>) {
  if (action.modal?.type === 'custom') {
    const Component = action.modal.component;
    return <Component record={record} revision={revision} onClose={onClose} />;
  }
  if (isConfirmAction(action)) {
    return <ConfirmDispatcher action={action} record={record} onClose={onClose} />;
  }
  return null;
}

// `action.modal?.type === 'confirm'` narrows `action.modal` to ConfirmModalConfig
// but doesn't eliminate the `{ modal?: never }` branch from ActionConfig<T>'s union,
// so `action` still types as the full union when passed to ConfirmDispatcher.
function isConfirmAction<T extends Data>(
  action: ActionConfig<T>
): action is ActionConfig<T> & {
  operation: MutationActionConfig | RouteActionConfig;
  modal: ConfirmModalConfig;
} {
  return action.modal?.type === 'confirm';
}
