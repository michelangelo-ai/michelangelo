import type { ReactNode } from 'react';
import type { BoxOverrides } from '#core/components/box/types';

interface FormGroupBaseProps {
  title?: string;

  /** Text displayed below title, **Markdown supported** */
  description?: string;

  /** Help tooltip text, displayed next to title. **Markdown supported** */
  tooltip?: string;

  /** Additional content for the header (e.g., action buttons) */
  endEnhancer?: ReactNode;

  /** BaseUI overrides for the underlying Box container */
  overrides?: BoxOverrides;

  children: ReactNode;
}

interface StaticFormGroupProps extends FormGroupBaseProps {
  collapsible?: false;
  expanded?: never;
  onToggle?: never;
}

interface CollapsibleFormGroupProps extends FormGroupBaseProps {
  /** Enables collapsing the group to hide its children */
  collapsible: true;

  /** Controlled expanded state — requires `onToggle` to respond to user interaction */
  expanded?: boolean;

  /** Called when the collapsible group is toggled */
  onToggle?: (expanded: boolean) => void;
}

export type FormGroupProps = StaticFormGroupProps | CollapsibleFormGroupProps;
