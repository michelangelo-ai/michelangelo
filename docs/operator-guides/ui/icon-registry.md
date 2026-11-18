# Icon Registry

Michelangelo AI Studio does not ship icon artwork. It ships icon *slots*: components
reference icons by name, and you supply the components that render them. This guide
covers what you must supply, the contract those components have to satisfy, and one
non-obvious behaviour — your icons also replace icons inside BaseWeb components.

**Audience:** anyone embedding `@michelangelo-ai/core` in a React application, or
operating a deployment that customizes Studio's look. If you only deploy the prebuilt
UI image, the reference registry is already wired up and you can skip this page.

## Supplying the registry

Pass an icon map on `dependencies.theme.icons`:

```tsx
import MichelangeloStudio from '@michelangelo-ai/core';

const customIcons = {
  arrowLaunch: YourLaunchIcon,
  circleCheckFilled: YourCheckIcon,
  // ...one entry per key in "Icons Studio looks up" below
};

function App() {
  return <MichelangeloStudio dependencies={{ theme: { icons: customIcons }, /* ... */ }} />;
}
```

Keys are **camelCase**. Values are React components. Studio resolves them by key at
render time.

## The icon component contract

An icon component must accept BaseUI's `IconProps` shape, minus `icon` and `overrides`:

```ts
type IconComponent = React.ComponentType<Omit<IconProps, 'icon' | 'overrides'>>;
```

The properties that matter in practice are `size` (a CSS length **string**, not a
number), `color` (a CSS color string), and `title`. Studio's `Icon` wrapper supplies
all three — `size` from the theme, `color` derived from the icon's `kind`.

This is why an icon library's components usually cannot be dropped in directly. A
Material UI icon, for example, expects `fontSize` and `htmlColor` rather than `size`
and `color`. The reference app bridges the gap with a small adapter
(`javascript/app/icons/mui-icon-adapter.tsx`) that translates props and drops the
BaseUI-only ones:

```tsx
export const createMuiIconAdapter = (Icon: ComponentType<SvgIconProps>) => {
  return (props: IconProps) => {
    const { size, style, color, shapeRendering, title, ...rest } = props;
    const { overrides: _o, fontSize: _f, ...compatible } = rest;
    return (
      <Icon
        {...compatible}
        htmlColor={color}
        shapeRendering={String(shapeRendering)}
        titleAccess={title ? String(title) : undefined}
        sx={{ ...style, fontSize: size ? `calc(${size} * 1.125)` : size }}
      />
    );
  };
};
```

Write the equivalent adapter for whichever library you use. If your components already
take `size`/`color` strings, you can register them directly.

:::warning
Your icons also replace icons inside BaseWeb components. Read
[Registry entries leak into BaseWeb](#registry-entries-leak-into-baseweb) before
choosing key names — this is the part that causes surprises.
:::

## Icons Studio looks up

These keys are referenced by name in shipped Studio code. Anything you leave out
renders as nothing (see [Unregistered keys fail silently](#unregistered-keys-fail-silently)).

| Key | Where it renders |
|---|---|
| `arrowCircular` | Run entity — **Retry** action |
| `arrowLaunch` | Page header — open-external link |
| `arrowLeft` | Table filter menu — back within the menu |
| `calendarRepeat` | Pipeline entity action |
| `chartLine` | Train phase icon |
| `check` | Deployment entity action |
| `chevronRight` | Breadcrumb menu drawer |
| `circleCheckFilled` | Boolean table cell — true value |
| `circleExclamation` | Deployment entity; form banner (error kind) |
| `circleI` | Help tooltip; form banner (info kind) |
| `circleX` | Deployment entity action |
| `deleteAlt` | Array form row — remove row |
| `lightbulb` | Retrain phase icon |
| `menu` | Breadcrumb menu drawer |
| `monitor` | Monitor & Debug phase icon |
| `overflowMenu` | Row action popover |
| `pencil` | Deployment entity — edit |
| `playerPlay` | Pipeline entity action |
| `plus` | Form add-button |
| `search` | Table search input |
| `settings` | Table column configuration |
| `stopCircle` | Trigger entity action |
| `trashCan` | Key-value form row — remove entry |

Studio also resolves icon names dynamically — from entity and phase configuration, table
column definitions, and action definitions (`<Icon name={action.display.icon} />`). Any
key you use in your own configuration must be registered too, and the list above will
not tell you about those.

The reference registry (`javascript/app/icons/icons.tsx`) additionally registers
`chevronDown`, `chevronUp`, `circleCheck`, `close`, `diamondEmpty`, `playerNext`,
`sortAscending`, `sortDescending`, `stars`, and `x`, which are reached only through
those dynamic paths.

## Registry entries leak into BaseWeb

This is the behaviour worth understanding before you name anything.

Studio passes your icon map to **two** providers. One is its own registry. The other is
the BaseWeb theme:

```tsx
<ThemeProvider icons={dependencies.theme.icons}>
  <IconProvider icons={dependencies.theme.icons}>
```

On the way into the theme, every key is capitalized:

```tsx
const iconEntries = Object.fromEntries(
  Object.entries(icons).map(([key, value]) => [capitalizeFirstLetter(key), value])
);
const resolvedTheme = createTheme({ ...GRID_OVERRIDES, icons: iconEntries });
```

BaseWeb resolves its own internal icons from exactly that map. Each BaseWeb icon module
does the equivalent of:

```js
component: theme.icons && theme.icons.ArrowLeft ? theme.icons.ArrowLeft : null
```

So registering `arrowLeft` for Studio also replaces the arrow BaseWeb draws inside its
own components — including components you never configured.

**Keys in the reference registry that currently do this:**

| Registry key | Becomes | Also replaces the icon in |
|---|---|---|
| `arrowLeft` | `ArrowLeft` | App nav bar, helper steps |
| `check` | `Check` | Progress steps, helper steps |
| `chevronDown` | `ChevronDown` | Accordion, app nav bar, data table, datepicker |
| `chevronRight` | `ChevronRight` | Breadcrumbs, datepicker, pagination, tree view |
| `chevronUp` | `ChevronUp` | App nav bar, data table, side navigation, table |
| `circleCheckFilled` | `CircleCheckFilled` | File uploader |
| `deleteAlt` | `DeleteAlt` | Input clear button, select clear button |
| `menu` | `Menu` | App nav bar |
| `plus` | `Plus` | — (no current consumer) |
| `search` | `Search` | Data table, select |

Mostly this is desirable: one `chevronRight` keeps disclosure arrows consistent. The
trap is that it is **implicit and name-driven**. A key chosen for a Studio button can
silently change an unrelated BaseWeb component, and nothing warns you.

Two practical consequences:

- **Adding an icon can change pages you did not touch.** Before adding a key, capitalize
  it and check it against BaseWeb's icon set. A new `delete` key becomes `Delete` and
  replaces BaseWeb's delete icon everywhere.
- **Prefer names BaseWeb does not use** when an icon is meant for Studio only. The
  reference registry does this — `trashCan` rather than `delete`, `overflowMenu` rather
  than `overflow`, `playerPlay` rather than `triangleRight`.

:::note
The camelCase-to-PascalCase bridge exists because the two systems disagree on
convention: Studio uses camelCase, BaseWeb uses PascalCase. Standardizing on one is
tracked in
[#364](https://github.com/michelangelo-ai/michelangelo/issues/364), and the code
carries a matching `TODO`. Until that lands, treat the capitalization as load-bearing —
it is what determines which BaseWeb icons you override.
:::

## Unregistered keys fail silently

If a name is not in the registry, `Icon` renders `null`. No console warning, no
placeholder, no error — the icon is simply absent, and layout closes up around it.

This is easy to hit and hard to diagnose, because it looks like a styling bug rather
than a missing registration. It is also live in the reference app today: the Deploy
phase (`config/phases/deploy.ts`) is `state: 'active'` and declares `icon: 'deploy'`,
but the reference registry has no `deploy` key, so that phase renders without one. The
disabled Data phase has the same gap with `database`.

When an icon is missing, check registration before styling.

## Passing a component directly

`Icon` also accepts a component instead of a name. When both are given, the component
wins and the name is ignored:

```tsx
<Icon name="alert" icon={Alert} />  // renders Alert; "alert" is not consulted
```

This bypasses the registry entirely, so it also bypasses the BaseWeb interaction above.
It is useful for one-off icons that should not become part of the shared registry.

## Related

- [Michelangelo AI React Library](./michelangelo-react-library.md) — embedding Studio,
  including where the icon map fits into the full `dependencies` object
- [Deploying Michelangelo AI UI](./deploying-michelangelo-ui.md) — deploying the
  prebuilt UI, which ships the reference registry
- [Local Development Setup](./local-development-setup.md) — running the UI locally to
  check icon changes
