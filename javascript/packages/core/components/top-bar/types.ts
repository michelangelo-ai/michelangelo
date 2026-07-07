import type { TopBarLink } from '#core/constants/types';

export type TopBarProps = {
  /** Optional. Adopters can point Docs/Help (or any custom link) at their own pages. */
  links?: TopBarLink[];
};
