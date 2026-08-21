import { LiveWorkspaces } from "./workspaces-live";
import visual from "../visual-dashboard.module.css";

export default function WorkspacesPage() {
  return <section className={visual.page}><header className={visual.head}><div className={visual.headMeta}><h1>Workspaces</h1><span className={visual.online}>Online</span></div></header><LiveWorkspaces /></section>;
}
