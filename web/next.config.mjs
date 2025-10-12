/** @type {import('next').NextConfig} */
const nextConfig = {
  eslint: {
    // Completely disable ESLint during build
    ignoreDuringBuilds: true,
  },
  typescript: {
    // Ignore TypeScript errors during build
    ignoreBuildErrors: true,
  },
};

export default nextConfig;
