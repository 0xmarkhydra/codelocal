import { Suspense } from "react";
import { LiveCodeGraph } from "./code-graph-live";
import visual from "../visual-dashboard.module.css";

export default function CodeGraphPage() {
  return <section className={visual.page}><header className={visual.head}><div className={visual.headMeta}><h1>Code Graph</h1><span className={visual.online}>Local</span></div></header><Suspense fallback={<section className={visual.panel}>Loading…</section>}><LiveCodeGraph /></Suspense></section>;
}
