import { LiveKnowledgeGraph } from "./knowledge-live";
import visual from "../visual-dashboard.module.css";

export default function KnowledgePage() {
  return <section className={visual.page}><header className={visual.head}><div className={visual.headMeta}><h1>Knowledge</h1><span className={visual.online}>Live</span></div></header><LiveKnowledgeGraph /></section>;
}
