/**
 * @fileoverview Disallows barrel exports in index files.
 *
 * Barrel files (index.ts that re-export from other modules) obscure where code
 * actually lives, make tree-shaking harder, and create circular-dependency
 * risks. Import directly from the source file instead.
 */

/** @type {import('eslint').Rule.RuleModule} */
const rule = {
  meta: {
    type: 'suggestion',
    docs: {
      description: 'Disallow barrel exports (re-exports) in index files',
      recommended: true,
    },
    messages: {
      noBarrelExport: [
        'Barrel exports are not allowed in index files.',
        'Import directly from the source file instead of re-exporting through an index.',
        '',
        '  // Instead of: export { Foo } from "./foo";',
        '  // Import directly: import { Foo } from "./components/foo";',
        '',
      ].join('\n'),
    },
    schema: [],
  },

  create(context) {
    const filename = context.getPhysicalFilename?.() ?? context.filename;
    const basename = filename.split('/').pop() ?? '';

    // Only apply to index files
    if (!/^index\.[tj]sx?$/.test(basename)) {
      return {};
    }

    return {
      ExportAllDeclaration(node) {
        context.report({ node, messageId: 'noBarrelExport' });
      },

      ExportNamedDeclaration(node) {
        if (node.source !== null) {
          context.report({ node, messageId: 'noBarrelExport' });
        }
      },
    };
  },
};

export default rule;
