// @nuxt/eslint flat config for Nuxt 4
// https://eslint.nuxt.com/docs
import { createConfigForNuxt } from '@nuxt/eslint-config/flat'

export default createConfigForNuxt({
  // Features
  features: {
    tooling: true,
    stylistic: false, // Prettier handles formatting
  },
  // Directories to lint
  dirs: {
    src: ['./app'],
  },
})
  // Custom overrides - use append to ensure these take precedence
  .append({
    rules: {
      // Allow single-word component names (many existing components use them)
      'vue/multi-word-component-names': 'off',
      // Relax v-html warning (project uses vue-dompurify-html)
      'vue/no-v-html': 'off',
      // Vue 3 allows multiple template roots
      'vue/no-multiple-template-root': 'off',
      // Allow unused vars with _ prefix convention
      '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
      // Allow explicit any (we have many legacy files)
      '@typescript-eslint/no-explicit-any': 'off',
      // Allow ts-ignore (legacy code uses it)
      '@typescript-eslint/ban-ts-comment': 'off',
      // Relax unified signatures (not worth enforcing on existing code)
      '@typescript-eslint/unified-signatures': 'off',
      // Disable regexp optimization rules (not real bugs)
      'regexp/no-super-linear-backtracking': 'off',
      'regexp/no-misleading-capturing-group': 'off',
      // Disable unicorn rules that conflict with existing code
      'unicorn/prefer-number-properties': 'off',
      // Allow empty catch blocks (legacy code)
      'no-empty': 'off',
      // Disable Vue rules that conflict with legacy code
      'vue/no-dupe-keys': 'off',
      'vue/no-parsing-error': 'off',
      'vue/no-deprecated-filter': 'off',
      'vue/valid-template-root': 'off',
      // Disable preserve-caught-error (not a standard rule)
      'preserve-caught-error': 'off',
    },
  })
