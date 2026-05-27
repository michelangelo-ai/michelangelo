import { MutationConfirmDispatcher } from './mutation-confirm-dispatcher';
import { RouteConfirmDispatcher } from './route-confirm-dispatcher';

import type { ActionConfig, Data } from './types';

type Props<T extends Data> = {
  action: ActionConfig<T>;
  record: T;
  onClose: () => void;
};

/**
 * Branches on the modal + action type and delegates to the appropriate
 * dispatcher. Each sub-dispatcher only calls the hooks it needs, so we
 * avoid rules-of-hooks violations from a single component branching on
 * config shape.
 */
export function ActionDispatcher<T extends Data>({ action, record, onClose }: Props<T>) {
  if (action.modal?.type === 'custom') {
    const Component = action.modal.component;
    return <Component record={record} onClose={onClose} />;
  }
  if (action.modal?.type === 'confirm' && action.action?.type === 'mutation') {
    return (
      <MutationConfirmDispatcher
        action={{ ...action, action: action.action, modal: action.modal }}
        record={record}
        onClose={onClose}
      />
    );
  }
  if (action.modal?.type === 'confirm' && action.action?.type === 'route') {
    return (
      <RouteConfirmDispatcher
        action={{ ...action, action: action.action, modal: action.modal }}
        onClose={onClose}
      />
    );
  }
  return null;
}
