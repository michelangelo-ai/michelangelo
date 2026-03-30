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
export type ActionConfigBase = {
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
  primary = 'primary',
  secondary = 'secondary',
  tertiary = 'tertiary',
}

export type Data = Record<string, unknown>;

export type ComponentActionConfig<T = Data> = ActionConfigBase & {
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
