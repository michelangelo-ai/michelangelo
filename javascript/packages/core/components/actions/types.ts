import type { BannerProps } from 'baseui/banner';
import type { Size as DialogSize } from 'baseui/dialog';
import type { ComponentType, ReactNode } from 'react';
import type { MiddlewareSchema } from '#core/hooks/use-schema-middleware/types';
import type { DeepInterpolatable } from '#core/interpolation/types';
import type { MutationConfig } from '#core/types/query-types';

export type Data = Record<string, unknown>;

/**
 * An action exposed on an entity (e.g. a row's overflow menu, a header button).
 *
 * Two shapes:
 * - **Action + optional confirm modal** — dispatches a mutation or route, optionally
 *   gated by a confirm dialog.
 * - **Custom modal** — opens a custom React component (a form, a wizard, etc.).
 *   Cannot pair with `action`; the component owns its own submit flow.
 */
export type ActionConfig<T = Data> = ActionConfigBase &
  (
    | { action: MutationActionConfig | RouteActionConfig; modal?: ConfirmModalConfig }
    | { modal: CustomModalConfig<T>; action?: never }
  );

export type ActionConfigBase = {
  /** Controls how the action's trigger button is displayed to the user. */
  display: ActionTriggerDisplay;

  /**
   * Visual hierarchy of the action's trigger button.
   * Actions without an explicit hierarchy default to tertiary (overflow menu).
   */
  hierarchy?: ActionHierarchy;

  /**
   * Optional rules to disable this action for specific records.
   * Rules are evaluated in order; the first match disables the item and
   * shows its message as a hover tooltip.
   */
  disabled?: DisabledRule[];
};

/** Action that fires a mutation against the API, with optional middleware to shape the record first. */
export type MutationActionConfig = {
  type: 'mutation';
  mutation: MutationConfig;
  middleware?: MiddlewareSchema;
};

export type RouteActionConfig = {
  type: 'route';
  route: string;
};

/** Confirm dialog gating a mutation or route action. */
export type ConfirmModalConfig = {
  type: 'confirm';
  header: { title: string };
  body?: ReactNode;
  banner?: BannerConfig;
  button: { label: string; icon?: string };
  /** Renders the confirm button in red. Use for irreversible actions (e.g. delete). */
  destructive?: boolean;
  size?: DialogSize;
};

/** Custom React component opened in a modal — owns its own submit flow. */
export type CustomModalConfig<T> = {
  type: 'custom';
  component: ComponentType<ActionComponentProps<T>>;
};

export type BannerConfig = {
  content: ReactNode;
  kind?: BannerProps['kind'];
  icon?: string;
};

/** Props passed to a component rendered by {@link CustomModalConfig}. */
export type ActionComponentProps<T = Data> = {
  record: T;
  onClose: () => void;
};

/**
 * Schema version of {@link ActionConfig} — what config authors write.
 * All leaf fields accept interpolated values via {@link DeepInterpolatable}.
 *
 * Components always receive the resolved {@link ActionConfig} — interpolation
 * is resolved at the per-row boundary before reaching any rendering code.
 */
export type ActionConfigSchema<T = Data> = DeepInterpolatable<ActionConfig<T>>;

export enum ActionHierarchy {
  PRIMARY = 'primary',
  SECONDARY = 'secondary',
  TERTIARY = 'tertiary',
}

/**
 * Item shape consumed by action renderers (see `useResolvedActionItems`).
 * Only display + `onClick` — no action-type knowledge. The `onClick`
 * delegates to whatever `onSelect` the renderer provided when the item
 * was resolved.
 */
export type ResolvedActionItem = {
  display: ActionTriggerDisplay;
  hierarchy?: ActionHierarchy;
  /** True if any of the source action's disabled rules matched. */
  disabled: boolean;
  /** Tooltip shown on hover/keyboard navigation when `disabled` is true. */
  disabledMessage?: string;
  onClick: () => void;
};

type ActionTriggerDisplay = {
  label: string;
  icon?: string;
};

/** A condition that disables an action for a specific record, with an optional hover tooltip. */
type DisabledRule = {
  condition: boolean;
  message?: string;
};
