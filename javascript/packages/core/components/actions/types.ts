import type { ComponentType } from 'react';

export type ActionConfig<T = Data> = ComponentActionConfig<T>;

/**
 * Base fields shared by all action configurations.
 *
 * @example
 * ```ts
 * const deleteAction: ComponentActionConfig<Pipeline> = {
 *   display: { label: 'Delete', icon: 'trash' },
 *   component: DeleteDialog,
 * };
 * ```
 */
export type ActionConfigBase<T = Data> = {
  /**
   * Controls how the action's trigger button is displayed to the user.
   *
   * @see {@link ActionTriggerDisplay}
   */
  display: ActionTriggerDisplay;

  /**
   * Visual hierarchy of the action's trigger button
   *
   * @note Actions without an explicit hierarchy default to tertiary (overflow menu).
   *
   * @see {@link ActionHierarchy}
   */
  hierarchy?: ActionHierarchy;

  /**
   * Optional rules to disable this action for specific records.
   * Rules are evaluated in order; the first match disables the item and
   * shows its message as a hover tooltip.
   */
  disabled?: DisabledRule<T>[];
};

/**
 * How the action's trigger button is displayed to the user
 *
 * @note icon is a string reference to an icon in the icon provider
 */
type ActionTriggerDisplay = {
  label: string;
  icon?: string;
};

export enum ActionHierarchy {
  PRIMARY = 'primary',
  SECONDARY = 'secondary',
  TERTIARY = 'tertiary',
}

export type Data = Record<string, unknown>;

export type ComponentActionConfig<T = Data> = ActionConfigBase<T> & {
  component: ComponentType<ActionComponentProps<T>>;
};

export type ActionComponentProps<T = Data> = {
  record: T;
  isOpen: boolean;
  onClose: () => void;
};

export type SelectedAction = {
  component: ComponentType<ActionComponentProps>;
  record: Data;
};

/** A condition that disables an action for a specific record, with an optional hover tooltip. */
type DisabledRule<T = Data> = {
  condition: (record: T) => boolean;
  message?: string;
};
