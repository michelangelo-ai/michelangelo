import { useState } from 'react';
import { get } from 'lodash';

import { getAllTableUserSettings, updateUserTableSettings } from './utils';

import type { Dispatch, SetStateAction } from 'react';

/**
 * Custom hook to manage and persist individual pieces of table state.
 *
 * Ensures that table state is persisted to localStorage and can be restored
 * when the component remounts.
 *
 * @param id - A unique identifier for the state. This corresponds to the localStorage path
 * @param defaultValue - The default value for the state
 * @returns Tuple of [currentState, setStateFunction]
 */
export function usePersistedTableState<StateType>(
  id: string,
  defaultValue: StateType
): [StateType, Dispatch<SetStateAction<StateType>>] {
  const settings = getAllTableUserSettings();
  // lodash.get can return Partial<StateType> so we need to cast to StateType
  const [state, setState] = useState<StateType>(get(settings, id, defaultValue) as StateType);

  const updateAndPersistState = (updater: SetStateAction<StateType>) => {
    const newStateValue = updater instanceof Function ? updater(state) : updater;
    updateUserTableSettings(id, newStateValue);
    setState(newStateValue);
  };

  return [state, updateAndPersistState];
}
