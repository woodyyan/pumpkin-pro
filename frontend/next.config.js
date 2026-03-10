/** @type {import('next').NextConfig} */
const backendApiUrl = (process.env.BACKEND_API_URL || 'http://localhost:8080').replace(/\/$/, '');

const nextConfig = {
  reactStrictMode: true,
  swcMinify: true,
  async rewrites() {
    return [
      {
        source: '/api/:path*',
        destination: `${backendApiUrl}/api/:path*`,
      },
    ];
  },
}

module.exports = nextConfig

