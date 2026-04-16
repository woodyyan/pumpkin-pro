/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  swcMinify: true,

  // Only treat .js / .jsx as page routes — never .test.js / .spec.js
  // This prevents Next.js from building test files as server-rendered pages.
  pageExtensions: ['js', 'jsx'],

  // ── Test-file exclusion for webpack builds ────────────────────────
  //
  // Test files import from 'node:test' / 'node:assert/strict'.
  // Webpack IgnorePlugin blocks them at compile time (resource stage).
  webpack: (config) => {
    config.plugins.push(
      new (require('webpack').IgnorePlugin)({
        resourceRegExp: /^node:/,
      }),
    );
    return config;
  },
};

module.exports = nextConfig;
