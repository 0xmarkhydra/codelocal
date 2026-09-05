import { DashboardShell } from "./dashboard-shell";

export default function DashboardLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <DashboardShell>{children}</DashboardShell>;
}

