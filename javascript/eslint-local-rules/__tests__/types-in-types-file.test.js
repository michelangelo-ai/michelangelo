import { RuleTester } from 'eslint';
import tseslint from 'typescript-eslint';

import rule from '../types-in-types-file.js';

RuleTester.describe = describe;
RuleTester.it = it;

const tester = new RuleTester({
  languageOptions: {
    ecmaVersion: 2020,
    sourceType: 'module',
    parser: tseslint.parser,
  },
});

tester.run('types-in-types-file', rule, {
  valid: [
    // ── types.ts files are always allowed ──
    {
      name: 'interface in types.ts',
      filename: 'types.ts',
      code: `export interface Foo { bar: string; }`,
    },
    {
      name: 'type alias in types.ts',
      filename: 'types.ts',
      code: `export type Foo = { bar: string };`,
    },

    // ── *-types.ts files are allowed ──
    {
      name: 'interface in a *-types.ts file',
      filename: 'column-types.ts',
      code: `export interface ColumnDef { name: string; }`,
    },

    // ── files in types/ directory are allowed ──
    {
      name: 'interface in a types/ directory',
      filename: '/src/components/types/view-types.ts',
      code: `export interface ViewConfig { layout: string; }`,
    },

    // ── Props interfaces used as function params are allowed ──
    {
      name: 'Props interface used in function declaration',
      filename: 'component.tsx',
      code: `
        interface MyComponentProps { label: string; }
        export function MyComponent(props: MyComponentProps) { return null; }
      `,
    },
    {
      name: 'Props interface used with destructured param',
      filename: 'component.tsx',
      code: `
        interface ButtonProps { onClick: () => void; }
        function Button({ onClick }: ButtonProps) { return null; }
      `,
    },
    {
      name: 'Props type used in arrow function',
      filename: 'component.tsx',
      code: `
        type CardProps = { title: string; };
        const Card = (props: CardProps) => null;
      `,
    },
    {
      name: 'Props type used in forwardRef generic',
      filename: 'component.tsx',
      code: `
        type ItemProps = { label: string; };
        const Item = forwardRef<HTMLDivElement, ItemProps>((props, ref) => null);
      `,
    },

    // ── re-exports are allowed (no declaration) ──
    {
      name: 're-export type from types file',
      filename: 'component.tsx',
      code: `export type { Foo } from './types';`,
    },

    // ── non-type exports are fine ──
    {
      name: 'exported function in a component file',
      filename: 'utils.ts',
      code: `export function foo() { return 1; }`,
    },
  ],

  invalid: [
    // ── local types in non-types files ──
    {
      name: 'local interface in component file',
      filename: 'component.tsx',
      code: `interface SomeHelper { key: string; }`,
      errors: [{ messageId: 'typesInTypesFile', data: { name: 'SomeHelper' } }],
    },
    {
      name: 'local type alias in component file',
      filename: 'utils.ts',
      code: `type MyType = string | number;`,
      errors: [{ messageId: 'typesInTypesFile', data: { name: 'MyType' } }],
    },

    // ── exported types in non-types files ──
    {
      name: 'exported interface in component file',
      filename: 'component.tsx',
      code: `export interface SomeHelper { key: string; }`,
      errors: [{ messageId: 'typesInTypesFile', data: { name: 'SomeHelper' } }],
    },
    {
      name: 'exported type alias in component file',
      filename: 'utils.ts',
      code: `export type MyType = string | number;`,
      errors: [{ messageId: 'typesInTypesFile', data: { name: 'MyType' } }],
    },

    // ── Props name without function param usage ──
    {
      name: 'Props interface NOT used as function param',
      filename: 'component.tsx',
      code: `
        interface FooProps { label: string; }
        export function MyComponent() { return null; }
      `,
      errors: [{ messageId: 'typesInTypesFile', data: { name: 'FooProps' } }],
    },

    // ── non-Props name used as function param ──
    {
      name: 'non-Props interface used as function param',
      filename: 'component.tsx',
      code: `
        interface Config { label: string; }
        export function MyComponent(props: Config) { return null; }
      `,
      errors: [{ messageId: 'inlineType', data: { name: 'Config' } }],
    },

    // ── mixed: one valid Props + one invalid non-Props ──
    {
      name: 'mixed: Props used as param (valid) + helper type (invalid)',
      filename: 'component.tsx',
      code: `
        interface MyComponentProps { label: string; }
        interface HelperType { key: string; }
        export function MyComponent(props: MyComponentProps) { return null; }
      `,
      errors: [{ messageId: 'typesInTypesFile', data: { name: 'HelperType' } }],
    },

    // ── inlineType: small type alias (≤2 members) used once ──
    {
      name: 'small type alias used once → inlineType',
      filename: 'utils.ts',
      code: `
        type Options = { verbose: boolean; limit: number };
        function run(opts: Options) { return opts; }
      `,
      errors: [{ messageId: 'inlineType', data: { name: 'Options' } }],
    },

    // ── inlineType: small interface (≤2 members) used once ──
    {
      name: 'small interface used once → inlineType',
      filename: 'utils.ts',
      code: `
        interface Pair { a: string; b: string }
        function merge(p: Pair) { return p; }
      `,
      errors: [{ messageId: 'inlineType', data: { name: 'Pair' } }],
    },

    // ── typesInTypesFile: small type used twice ──
    {
      name: 'small type used twice → typesInTypesFile',
      filename: 'utils.ts',
      code: `
        type Tag = { label: string };
        function make(t: Tag) { return t; }
        function read(t: Tag) { return t; }
      `,
      errors: [{ messageId: 'typesInTypesFile', data: { name: 'Tag' } }],
    },

    // ── typesInTypesFile: large type alias (3+ members) used once ──
    {
      name: 'large type alias used once → typesInTypesFile',
      filename: 'utils.ts',
      code: `
        type BigConfig = { a: string; b: number; c: boolean };
        function init(cfg: BigConfig) { return cfg; }
      `,
      errors: [{ messageId: 'typesInTypesFile', data: { name: 'BigConfig' } }],
    },

    // ── inlineType: small union (≤2 members) used once ──
    {
      name: 'small union used once → inlineType',
      filename: 'utils.ts',
      code: `
        type Status = 'on' | 'off';
        function toggle(s: Status) { return s; }
      `,
      errors: [{ messageId: 'inlineType', data: { name: 'Status' } }],
    },
  ],
});
