import { LiveUsage } from "./usage-live";
import visual from "../visual-dashboard.module.css";

export default function UsagePage() {
  return <section className={visual.page}><header className={visual.head}><div className={visual.headMeta}><h1>Usage</h1><span className={visual.online}>Live</span></div></header><LiveUsage /></section>;
}
