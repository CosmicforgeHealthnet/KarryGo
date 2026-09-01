// next.config.ts

import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      {
        source: '/api/:path*',
        destination: 'http://localhost:8113/fleet-partner/v1/:path*',
      },
    ];
  },
};

export default nextConfig;