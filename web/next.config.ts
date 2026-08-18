import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  turbopack: {
    // This repository intentionally has a separate root lockfile for the Go/npm
    // distribution. Keep Next.js module resolution and file watching scoped to
    // the browser product instead of inferring the monorepo root.
    root: process.cwd(),
  },
};

export default nextConfig;
