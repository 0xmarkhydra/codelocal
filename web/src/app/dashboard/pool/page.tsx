import dashboard from "../dashboard.module.css";
import { PoolLive } from "./pool-live";

export default function PoolPage() {
  return (
    <section className={dashboard.content}>
      <header className={dashboard.header}>
        <div>
          <span className={dashboard.eyebrow}>Model gateway</span>
          <h1>AI Pool</h1>
          <p>Quản lý gateway 9Router trung tâm cho Codex, Claude, Gemini và các API provider.</p>
        </div>
      </header>
      <PoolLive />
    </section>
  );
}
