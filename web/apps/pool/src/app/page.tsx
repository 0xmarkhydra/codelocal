import { redirect } from "next/navigation";
import { hasPoolSession } from "@/lib/server/session";
import { PoolDashboard } from "./pool-dashboard";

export default async function HomePage() {
  if (!(await hasPoolSession())) redirect("/login");
  return <PoolDashboard />;
}
