import { LiveOverview } from "./overview-live";
import styles from "./dashboard.module.css";

const migration = [
  { label: "Web foundation", state: "Ready", note: "Next.js App Router, TypeScript, design tokens and shared dashboard layout." },
  { label: "Overview", state: "Ready", note: "Validated read-only Go overview DTO with honest unavailable/unauthenticated states." },
  { label: "Workspaces + Devices + Usage", state: "Ready", note: "Read-only route families use minimal client-neutral DTOs, runtime validation and shared resource loading." },
  { label: "Session bridge", state: "Implemented", note: "Same-origin browser API proxy is in place without replaying HttpOnly session secrets in Next." },
  { label: "Deployment parity", state: "Next", note: "Verify Set-Cookie, browser User-Agent and trusted client-IP forwarding on the real edge before cutover." },
  { label: "Remaining routes", state: "Queued", note: "Knowledge, Code Graph, Security, Account and mutations remain on Go until individually migrated." },
];

export default function DashboardPage() {
  return (
    <section className={styles.content}>
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Next.js migration</span>
          <h1>Overview</h1>
          <p>New browser architecture without changing the current production authority model.</p>
        </div>
        <span className={styles.productionLink}>Read-only overview · Go authority</span>
      </header>

      <LiveOverview />

      <section className={styles.heroPanel}>
        <div>
          <span className={styles.eyebrow}>Migration guardrails</span>
          <h2>Next renders the product surface. Go still owns identity, authorization and local/cloud truth.</h2>
          <p>
            Authenticated overview reads use a same-origin versioned API proxy so HttpOnly session cookies stay browser-managed.
            Mutations and route families that have not reached parity continue to fall back to the existing Go application.
          </p>
        </div>
        <div className={styles.boundaryDiagram} aria-label="Web and backend ownership diagram">
          <span>Next.js web</span>
          <b>HTTP contracts</b>
          <span>Go backend</span>
          <b>authenticated runtime</b>
          <span>local machine</span>
        </div>
      </section>

      <section className={styles.grid}>
        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <div>
              <span className={styles.eyebrow}>Migration track</span>
              <h3>Route-family rollout</h3>
            </div>
            <span className={styles.badge}>NO CUTOVER YET</span>
          </div>
          <div className={styles.migrationList}>
            {migration.map((item) => (
              <div className={styles.migrationRow} key={item.label}>
                <span className={styles.stateDot} data-state={item.state.toLowerCase().replaceAll(" ", "-")} />
                <div>
                  <strong>{item.label}</strong>
                  <p>{item.note}</p>
                </div>
                <span className={styles.state}>{item.state}</span>
              </div>
            ))}
          </div>
        </article>

        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <div>
              <span className={styles.eyebrow}>Authority map</span>
              <h3>What stays where</h3>
            </div>
          </div>
          <dl className={styles.authorityList}>
            <div><dt>Web presentation</dt><dd>Next.js / TypeScript</dd></div>
            <div><dt>Cloud authority</dt><dd>Go backend</dd></div>
            <div><dt>Local execution</dt><dd>Go runtime / CLI</dd></div>
            <div><dt>Desktop + mobile</dt><dd>Flutter / Dart</dd></div>
            <div><dt>macOS deep integration</dt><dd>Swift native bridge</dd></div>
          </dl>
        </article>
      </section>
    </section>
  );
}
