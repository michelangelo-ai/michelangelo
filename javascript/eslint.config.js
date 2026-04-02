// javascript/eslint.config.js
import js from '@eslint/js';

import noBarrelExports from './eslint-local-rules/no-barrel-exports.js';
import noFixtureConstants from './eslint-local-rules/no-fixture-constants.js';
import noModuleScopeTestSetup from './eslint-local-rules/no-module-scope-test-setup.js';
import tseslint from 'typescript-eslint';
import prettierConfig from 'eslint-config-prettier';
import baseUIEslint from 'eslint-plugin-baseui';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';
import simpleImportSort from 'eslint-plugin-simple-import-sort';
import globals from 'globals';

// Shared plugins (used in app and packages/*)
const sharedPlugins = {
  'react-hooks': reactHooks,
  'simple-import-sort': simpleImportSort,
  'react-refresh': reactRefresh,
  baseui: baseUIEslint,
};

// Shared rules (used in app and packages/*)
const sharedRules = {
  ...reactHooks.configs.recommended.rules,
  'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
  'simple-import-sort/imports': [
    'error',
    {
      groups: [
        // Group 1: React and third-party imports (React first)
        ['^react', '^@?\\w', '^[^#./]'],

        // Group 2: Internal imports (#) and relative imports
        ['^#\\w+', '^\\.'],

        // Group 3: Type imports (both third-party and local)
        ['^@?\\w.*\\u0000$', '^[^.].*\\u0000$', '^\\..*\\u0000$'],

        // Group 4: Style imports
        ['^.*\\.(css|scss|sass|less)$'],
      ],
    },
  ],
  // Disabled due to BaseUI 15 compatibility issues
  // 'baseui/deprecated-theme-api': 'warn',
  // 'baseui/deprecated-component-api': 'warn',
  'no-nested-ternary': 'error',
  eqeqeq: ['error', 'always', { null: 'ignore' }],
  'no-restricted-syntax': [
    'error',
    {
      selector: 'ExportDefaultDeclaration',
      message:
        'Use named exports. Default exports make imports harder to refactor and autocomplete.',
    },
  ],
  'baseui/no-deep-imports': 'warn',
  '@typescript-eslint/array-type': 'off',
  '@typescript-eslint/consistent-type-definitions': 'off',
  '@typescript-eslint/no-unsafe-call': 'off',
  '@typescript-eslint/no-unused-vars': [
    'error',
    { argsIgnorePattern: '^_', varsIgnorePattern: '^_' },
  ],
  '@typescript-eslint/ban-ts-comment': [
    'error',
    {
      'ts-ignore': true,
      'ts-expect-error': 'allow-with-description',
      'ts-nocheck': true,
      'ts-check': false,
    },
  ],
  '@typescript-eslint/naming-convention': [
    'error',
    {
      selector: 'typeLike',
      format: ['PascalCase'],
      custom: { regex: 'T$', match: false },
    },
    {
      selector: 'typeParameter',
      format: ['PascalCase'],
    },
  ],
  '@typescript-eslint/consistent-type-imports': [
    'error',
    {
      prefer: 'type-imports',
      fixStyle: 'separate-type-imports',
      disallowTypeAnnotations: false,
    },
  ],
};

export default [
  {
    linterOptions: {
      reportUnusedDisableDirectives: 'error',
    },
  },

  {
    ignores: [
      '**/node_modules/**',
      '**/dist/**',
      '**/gen/**',
      '**/coverage/**',
      'eslint.config.js',
      '**/vite-env.d.ts',
      'eslint-local-rules/**',
    ],
  },

  js.configs.recommended,
  ...tseslint.configs.recommendedTypeChecked,
  ...tseslint.configs.stylisticTypeChecked,

  // App code
  {
    files: ['app/**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2020,
      sourceType: 'module',
      parser: tseslint.parser,
      parserOptions: {
        projectService: true,
      },
      globals: globals.browser,
    },
    plugins: sharedPlugins,
    rules: sharedRules,
  },

  // Vite config (Node)
  {
    files: ['app/vite.config.ts'],
    languageOptions: {
      ecmaVersion: 2020,
      sourceType: 'module',
      parser: tseslint.parser,
      parserOptions: {
        projectService: true,
      },
      globals: globals.node,
    },
  },

  // Core package tests
  {
    files: ['packages/core/**/__tests__/**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2020,
      sourceType: 'module',
      parser: tseslint.parser,
      parserOptions: {
        projectService: true,
      },
      globals: globals.browser,
    },
    plugins: sharedPlugins,
    rules: sharedRules,
  },

  // All package tests — enforce test setup conventions
  {
    files: ['packages/**/__tests__/**/*.{ts,tsx}'],
    plugins: {
      local: { rules: { 'no-module-scope-test-setup': noModuleScopeTestSetup } },
    },
    rules: {
      'local/no-module-scope-test-setup': 'error',
    },
  },

  // All package fixtures — enforce factory function exports only
  {
    files: ['packages/**/__fixtures__/**/*.{ts,tsx}'],
    plugins: {
      local: { rules: { 'no-fixture-constants': noFixtureConstants } },
    },
    rules: {
      'local/no-fixture-constants': 'error',
    },
  },

  // App and packages — no barrel exports in index files
  {
    files: ['packages/core/**/*.{ts,tsx}', 'packages/rpc/**/*.{ts,tsx}', 'app/**/*.{ts,tsx}'],
    ignores: [
      'packages/core/index.tsx',
      'packages/rpc/index.ts',
      'packages/**/__tests__/**/*.{ts,tsx}',
      'packages/**/__fixtures__/**/*.{ts,tsx}',
    ],
    plugins: {
      local: { rules: { 'no-barrel-exports': noBarrelExports } },
    },
    rules: {
      'local/no-barrel-exports': 'error',
    },
  },

  // Core package
  {
    files: ['packages/core/**/*.{ts,tsx}'],
    ignores: ['packages/core/**/__tests__/**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2020,
      sourceType: 'module',
      parser: tseslint.parser,
      parserOptions: {
        projectService: true,
      },
      globals: globals.browser,
    },
    plugins: sharedPlugins,
    rules: sharedRules,
  },

  // Interpolation module - Allow unsafe operations for dynamic data handling
  {
    files: [
      'packages/core/interpolation/**/*.{ts,tsx}',
      'packages/core/providers/interpolation-provider/**/*.{ts,tsx}',
    ],
    rules: {
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-member-access': 'off',
      '@typescript-eslint/no-unsafe-return': 'off',
      '@typescript-eslint/no-explicit-any': 'off',
    },
  },

  // RPC package
  {
    files: ['packages/rpc/**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2020,
      sourceType: 'module',
      parser: tseslint.parser,
      parserOptions: {
        projectService: true,
      },
      globals: globals.browser,
    },
    plugins: sharedPlugins,
    rules: sharedRules,
  },

  // Allow default exports in config files and type declarations (required by their frameworks)
  {
    files: ['vitest.config.ts', '**/vite.config.ts', '**/vite.config.production.ts', '**/*.d.ts'],
    rules: {
      'no-restricted-syntax': 'off',
    },
  },

  // vitest.config.ts is not included in any tsconfig discovered by projectService —
  // disable type-aware rules for this single config file
  {
    files: ['vitest.config.ts'],
    ...tseslint.configs.disableTypeChecked,
  },

  // Disable conflicting style rules (Prettier)
  prettierConfig,
];
