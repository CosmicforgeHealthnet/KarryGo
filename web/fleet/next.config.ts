import type { NextConfig } from "next";

const fleetApiBase =
  process.env.FLEET_API_BASE_URL ?? "http://localhost:8113";

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: `${fleetApiBase}/fleet-partner/v1/:path*`,
      },
      {
        source: "/fleet-partner/v1/:path*",
        destination: `${fleetApiBase}/fleet-partner/v1/:path*`,
      },
    ];
  },
};

export default nextConfig;