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
    | { operation: MutationActionConfig | RouteActionConfig; modal: ConfirmModalConfig }
    | { operation: MutationActionConfig | RouteActionConfig; modal?: never }
    | { modal: CustomModalConfig<T>; operation?: never }
  );

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

/** Action that fires a mutation against the API, with optional middleware to shape the record first. */
export type MutationActionConfig = {
  type: 'mutation';
  mutation: MutationConfig;
  middleware?: MiddlewareSchema;
  /** Side-effects to run after the mutation succeeds (in order). */
  successOperations?: SuccessOperation[];
};

/**
 * Side-effect to run after a mutation succeeds.
 *
 * Use `invalidate` to refresh related queries explicitly after a mutation
 * succeeds, either broadly by query name or narrowly by name + args.
 */
export type SuccessOperation = InvalidateOperation | ToastOperation;

export type InvalidateOperation = {
  type: 'invalidate';
  /** Each target invalidates queries by name only (broad) or by name+args (specific). */
  targets: InvalidationTarget[];
};

export type InvalidationTarget = string | { name: string; serviceOptions: Record<string, unknown> };

export type ToastOperation = {
  type: 'toast';
  message: string;
  /** Icon registered in the IconProvider. Defaults to 'checkCircle'. */
  icon?: string;
  /** Optional action button rendered inside the toast. */
  action?: {
    label: string;
    /** If set, clicking the action navigates to this route; otherwise it dismisses the toast. */
    route?: string;
  };
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
  /** Registered icon name from the icon provider. */
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
 * Item shape consumed by action renderers — display + `onClick` with no
 * action-type knowledge. The `onClick` is pre-bound by whoever resolved the items.
 */
export type ResolvedActionItem = {
  display: ActionTriggerDisplay;
  hierarchy?: ActionHierarchy;
  disabled: boolean;
  /** Tooltip shown on hover/keyboard navigation when `disabled` is true. */
  disabledMessage?: string;
  onClick: () => void;
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

/** A condition that disables an action for a specific record, with an optional hover tooltip. */
type DisabledRule = {
  condition: boolean;
  message?: string;
};
