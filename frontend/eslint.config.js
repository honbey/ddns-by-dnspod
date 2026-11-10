import js from '@eslint/js'
import prettier from 'eslint-config-prettier'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import globals from 'globals'
import tseslint from 'typescript-eslint'

/**
 * Linting covers what the type checker cannot see: a hook called from the wrong
 * place, a dependency list that lies, a module that exports both a component
 * and a constant and so defeats fast refresh. Formatting is Prettier's job, and
 * `eslint-config-prettier` goes last so the two never argue.
 *
 * Only the two classic hook rules are enabled. The React Compiler rules that
 * ship with the same plugin (`set-state-in-effect`, `incompatible-library`, ...)
 * are advisory rewrites of working patterns - a dialog that seeds its form from
 * a prop, TanStack Table - and turning them on here would mean either rewriting
 * every such component or silencing the rule per file.
 *
 * Generated files are ignored rather than reformatted: they are rewritten by
 * their generators, and CI checks that they still match the contract.
 */
export default tseslint.config(
  {
    ignores: [
      'dist',
      'node_modules',
      'src/api-types.gen.ts',
      'src/routeTree.gen.ts',
      'test-results',
      'playwright-report',
    ],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    // Build scripts and the Playwright harness run in Node, not the browser.
    // Flat config merges globals across matching blocks, so public/ has to be
    // excluded here or `process` would leak into the browser bootstrap's
    // no-undef scope.
    files: ['**/*.{js,mjs,cjs}'],
    ignores: ['public/**'],
    languageOptions: { globals: { ...globals.node } },
  },
  {
    // Vite copies public/ verbatim: the theme bootstrap is a classic browser
    // script with no module scope. Give it browser globals rather than taking
    // the whole directory out of every rule.
    files: ['public/theme-init.js'],
    languageOptions: { globals: { ...globals.browser } },
  },
  {
    files: ['e2e/**/*.ts', 'scripts/**/*.ts'],
    languageOptions: { globals: { ...globals.node } },
  },
  {
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2022,
      globals: { ...globals.browser, ...globals.node },
    },
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
      // A shipped page should not narrate itself. Errors and warnings are the
      // exception: there is nowhere else for them to go.
      'no-console': ['error', { allow: ['error', 'warn'] }],
      // A leading underscore is the langauge's way of saying "part of the
      // signature, not of the implementation": type parameters that exist only
      // to match a library's arity, rest siblings dropped on purpose.
      '@typescript-eslint/no-unused-vars': [
        'error',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_', ignoreRestSiblings: true },
      ],
    },
  },
  {
    // shadcn's primitives export the component together with its variant map
    // (`buttonVariants`, `badgeVariants`); that is the shape the generator
    // produces, so the fast-refresh rule has nothing to offer here.
    files: ['src/components/ui/**/*.{ts,tsx}'],
    rules: { 'react-refresh/only-export-components': 'off' },
  },
  prettier
)
