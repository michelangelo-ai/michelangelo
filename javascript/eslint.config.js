// javascript/eslint.config.js
import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import prettierConfig from 'eslint-config-prettier';
import baseUIEslint from 'eslint-plugin-baseui';
import prettierPlugin from 'eslint-plugin-prettier';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';
import simpleImportSort from 'eslint-plugin-simple-import-sort';
import globals from 'globals';

// Shared plugins (used in app and packages/*)
const sharedPlugins = {
  prettier: prettierPlugin,
  'react-hooks': reactHooks,
  'simple-import-sort': simpleImportSort,
  'react-refresh': reactRefresh,
  baseui: baseUIEslint,
};

// Shared rules (used in app and packages/*)
const sharedRules = {
  ...reactHooks.configs.recommended.rules,
  'prettier/prettier': 'error',
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
  'baseui/deprecated-theme-api': 'warn',
  'baseui/deprecated-component-api': 'warn',
  'baseui/no-deep-imports': 'warn',
  '@typescript-eslint/array-type': 'off',
  '@typescript-eslint/consistent-type-definitions': 'off',
  '@typescript-eslint/no-unsafe-call': 'off',
};

export default [
  {
    ignores: [
      '**/node_modules/**',
      '**/dist/**',
      '**/gen/**',
      'eslint.config.js',
      '**/vite-env.d.ts',
    ],
  },

  {
    files: ['app/vite.config.ts'],
    languageOptions: {
      parser: tseslint.parser,
      parserOptions: {
        project: new URL('./app/tsconfig.app.json', import.meta.url).pathname,
        tsconfigRootDir: new URL('./app', import.meta.url).pathname,
      },
      ecmaVersion: 2020,
      sourceType: 'module',
    },
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
        project: new URL('./app/tsconfig.app.json', import.meta.url).pathname,
        tsconfigRootDir: new URL('./app', import.meta.url).pathname,
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
        project: new URL('./app/tsconfig.node.json', import.meta.url).pathname,
        tsconfigRootDir: new URL('./app', import.meta.url).pathname,
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
        project: new URL('./packages/core/tsconfig.test.json', import.meta.url).pathname,
        tsconfigRootDir: new URL('./packages/core', import.meta.url).pathname,
      },
      globals: globals.browser,
    },
    plugins: sharedPlugins,
    rules: sharedRules,
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
        project: new URL('./packages/core/tsconfig.json', import.meta.url).pathname,
        tsconfigRootDir: new URL('./packages/core', import.meta.url).pathname,
      },
      globals: globals.browser,
    },
    plugins: sharedPlugins,
    rules: sharedRules,
  },

  // RPC package
  {
    files: ['packages/rpc/**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2020,
      sourceType: 'module',
      parser: tseslint.parser,
      parserOptions: {
        project: new URL('./packages/rpc/tsconfig.json', import.meta.url).pathname,
        tsconfigRootDir: new URL('./packages/rpc', import.meta.url).pathname,
      },
      globals: globals.browser,
    },
    plugins: sharedPlugins,
    rules: sharedRules,
  },

  // Disable conflicting style rules (Prettier)
  prettierConfig,
];
