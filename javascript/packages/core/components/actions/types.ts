import type { ComponentType } from 'react';

import type { DeepInterpolatable, Interpolatable } from '#core/interpolation/types';

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

  /**
   * Optional rules to disable this action for specific records.
   * Rules are evaluated in order; the first match disables the item and
   * shows its message as a hover tooltip.
   */
  disabled?: DisabledRule[];
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

/**
 * Schema version of {@link ActionConfig} — what config authors write.
 * All leaf fields accept interpolated values via {@link DeepInterpolatable}.
 *
 * Components always receive the resolved {@link ActionConfig} — interpolation
 * is resolved at the per-row boundary before reaching any rendering code.
 */
export type ActionConfigSchema<T = Data> = DeepInterpolatable<ActionConfig<T>>;

/** A condition that disables an action for a specific record, with an optional hover tooltip. */
type DisabledRule = {
  condition: Interpolatable<boolean>;
  message?: string;
};
