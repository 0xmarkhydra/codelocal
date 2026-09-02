import { poolClientProxy } from "@/lib/server/pool-api";

export async function POST(request: Request) {
  return poolClientProxy(request, "/v1/chat/completions");
}
