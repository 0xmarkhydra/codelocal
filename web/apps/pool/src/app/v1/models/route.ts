import { poolClientProxy } from "@/lib/server/pool-api";

export async function GET(request: Request) {
  return poolClientProxy(request, "/v1/models");
}
