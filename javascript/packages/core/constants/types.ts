/** A single top nav bar link: text shown to the user and the URL opened on click. */
export type TopBarLink = {
  label: string;
  url: string;
};

/** Adopter-configurable top nav bar links — replaces the Docs/Help defaults when provided. */
export type TopBarLinks = TopBarLink[];
