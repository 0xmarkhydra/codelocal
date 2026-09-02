import { poolHealth } from "@/lib/server/pool-api";

export async function GET() {
  return poolHealth();
}
