import type { ComponentType } from 'react';

export type ActionSchema<T = Data> = ComponentActionSchema<T>;

/**
 * Base fields shared by all action configurations.
 *
 * @example
 * ```ts
 * const deleteAction: ComponentActionSchema<Pipeline> = {
 *   display: { label: 'Delete', icon: 'trash' },
 *   component: DeleteDialog,
 * };
 * ```
 */
export type ActionSchemaBase = {
  display: {
    label: string;
    icon?: string;
  };
};

export type Data = Record<string, unknown>;

export type ComponentActionSchema<T = Data> = ActionSchemaBase & {
  component: ComponentType<ActionComponentProps<T>>;
};

export type ActionComponentProps<T = Data> = {
  record: T;
  isOpen: boolean;
  onClose: () => void;
};

export type ActionMenuContextValue = {
  closeMenu: () => void;
  openAction: (action: { component: ComponentType<ActionComponentProps>; record: Data }) => void;
};
export type ActionContextValue = { onSuccess?: (data: unknown) => void };
