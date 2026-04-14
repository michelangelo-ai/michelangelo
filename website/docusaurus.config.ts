import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

const config: Config = {
  title: 'Michelangelo',
  tagline: 'ML Platform Documentation',
  favicon: 'img/favicon.svg',

  future: {
    v4: true,
    experimental_faster: true,
  },

  // GitHub Pages project site config (revert to /michelangelo/ until custom domain is verified)
  url: 'https://michelangelo-ai.github.io',
  baseUrl: '/michelangelo/',
  organizationName: 'michelangelo-ai',
  projectName: 'michelangelo',

  // In CI lint mode, use 'warn' so all broken links are reported at once
  // rather than failing on the first one. The workflow fails the build after
  // annotating every broken link.
  onBrokenLinks: process.env.CI_LINT === 'true' ? 'warn' : 'throw',

  markdown: {
    format: 'md',
    hooks: {
      onBrokenMarkdownLinks: process.env.CI_LINT === 'true' ? 'warn' : 'throw',
    },
  },

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          path: '../docs',
          routeBasePath: '/docs',
          sidebarPath: './sidebars.ts',
          editUrl: 'https://github.com/michelangelo-ai/michelangelo/tree/main/website/',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    image: 'img/michelangelo-logo-color-text.svg',
    colorMode: {
      defaultMode: 'dark',
      disableSwitch: false,
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'Michelangelo',
      logo: {
        alt: 'Michelangelo Logo',
        src: 'img/logo.svg',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docsSidebar',
          position: 'left',
          label: 'Docs',
        },
        {
          href: 'https://github.com/michelangelo-ai/michelangelo',
          label: 'GitHub',
          position: 'right',
          className: 'header-github-link',
          'aria-label': 'GitHub repository',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Docs',
          items: [
            {
              label: 'Getting Started',
              to: '/docs',
            },
          ],
        },
        {
          title: 'More',
          items: [
            {
              label: 'GitHub',
              href: 'https://github.com/michelangelo-ai/michelangelo',
            },
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Michelangelo.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['go', 'python', 'bash', 'yaml', 'json'],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
