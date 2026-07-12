import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

export default defineConfig([
  globalIgnores([
    'dist',
    // Vendored Inspinia v5 theme code (the demo views under /demo/* plus a
    // handful of theme-shared modules) is kept as-is as reference and was
    // never lint-clean — it is excluded so `npm run lint` gates OUR code.
    // See docs/FRONTEND.md ("keep the demo on the side").
    'src/views/admin/**',
    'src/views/auth/**',
    'src/views/error/**',
    'src/views/landing/**',
    'src/views/others/**',
    'src/components/FileUploader.tsx',
    'src/components/cards/ComponentCard.tsx',
    'src/components/wrappers/BaseVectorMap.tsx',
    'src/components/wrappers/EChart.tsx',
    'src/context/useLayoutContext.tsx',
    'src/hooks/useScrollEvent.ts',
  ]),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
    },
  },
])
