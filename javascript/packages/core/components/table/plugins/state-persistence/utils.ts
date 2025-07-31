import { cloneDeep, get, isEqual, set } from 'lodash';

import { TABLE_LOCAL_STORAGE_KEY } from '#core/components/table/constants';
import { safeLocalStorageGetItem, safeLocalStorageSetItem } from '#core/utils/local-storage-utils';

import type { TableState } from '#core/components/table/types/table-types';

const INITIAL_STATE = {} as const;

export function getAllTableUserSettings(): Record<string, Partial<TableState>> {
  return safeLocalStorageGetItem(TABLE_LOCAL_STORAGE_KEY, INITIAL_STATE);
}

export function getTableUserSettings(tableId?: string): Partial<TableState> {
  if (tableId) {
    const allTableSettings = getAllTableUserSettings();
    const settings = allTableSettings[tableId];
    if (settings !== undefined) {
      return settings;
    }
  }
  return INITIAL_STATE;
}

/**
 * Updates user table settings in localStorage
 *
 * @param settingsId - The unique identifier for the table settings
 * @param newSettings - The new settings to be applied
 * @returns true if the settings were updated, false otherwise
 */
export function updateUserTableSettings<T = unknown>(settingsId: string, newSettings: T): boolean {
  const current = getAllTableUserSettings();
  if (isEqual(get(current, settingsId), newSettings)) {
    return false;
  }

  safeLocalStorageSetItem(
    TABLE_LOCAL_STORAGE_KEY,
    set(cloneDeep(current), settingsId, newSettings)
  );
  return true;
}
