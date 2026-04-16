/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  swcMinify: true,

  // ── Test-file exclusion for webpack builds ────────────────────────
  //
  // Test files under lib/__tests__/ and components/__tests__/ import
  // from 'node:test' / 'node:assert/strict' which triggers Webpack 5's
  // UnhandledSchemeError at the *resource-reading* stage (before resolve).
  // resolve.alias cannot catch this — we must use IgnorePlugin instead.
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
