"use client";

import { isAIPoolResource } from "@/lib/contracts/pool";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import { useDashboardResource } from "../use-dashboard-resource";
import styles from "./pool.module.css";

function statusCopy(configured: boolean, available: boolean, routingReady: boolean) {
  if (!configured) return { label: "Not configured", state: "offline" } as const;
  if (!available) return { label: "Gateway unavailable", state: "warning" } as const;
  if (!routingReady) return { label: "Needs default route", state: "warning" } as const;
  return { label: "Ready", state: "online" } as const;
}

export function PoolLive() {
  const { state, retry } = useDashboardResource("/api/v1/pool", isAIPoolResource);

  if (state.kind !== "ready") {
    return (
      <DashboardResourceFeedback
        label="AI Pool"
        {...(state.kind === "error"
          ? { kind: "error" as const, message: state.message, onRetry: retry }
          : { kind: state.kind })}
      />
    );
  }

  const pool = state.value;
  const status = statusCopy(pool.configured, pool.available, pool.routingReady);

  return (
    <div className={styles.shell}>
      <section className={styles.hero}>
        <div>
          <span className={styles.kicker}>9Router · central account pool</span>
          <h2>Một gateway trung tâm cho toàn bộ tài khoản AI.</h2>
          <p>
            Codex, Claude, Gemini và API provider được kết nối trực tiếp trên Pool. CodeLocal chỉ gọi
            endpoint bằng API key riêng và để 9Router xử lý account, quota, combo và fallback.
          </p>
          <div className={styles.flow} aria-label="AI Pool routing flow">
            <span>CodeLocal</span><i>→</i><span>AI Pool</span><i>→</i><span>9Router combo</span><i>→</i><span>Provider accounts</span>
          </div>
        </div>
        <div className={styles.heroActions}>
          <span className={styles.status} data-state={status.state}><i />{status.label}</span>
          {pool.dashboardUrl ? (
            <a href={pool.dashboardUrl} target="_blank" rel="noreferrer">Open Pool Dashboard ↗</a>
          ) : null}
        </div>
      </section>

      <section className={styles.metrics} aria-label="AI Pool status">
        <article><span>Gateway</span><strong>{pool.available ? "Online" : "Offline"}</strong></article>
        <article><span>Models discovered</span><strong>{pool.modelCount}</strong></article>
        <article><span>Auto route</span><strong title={pool.defaultModel}>{pool.defaultModel || "Not set"}</strong></article>
      </section>

      <div className={styles.grid}>
        <section className={styles.panel}>
          <header>
            <div>
              <span className={styles.kicker}>Provider catalog</span>
              <h3>Models exposed by the Pool</h3>
            </div>
            <button type="button" onClick={retry}>Refresh</button>
          </header>
          {!pool.configured ? (
            <div className={styles.empty}><strong>CodeLocal Cloud chưa cấu hình AI Pool.</strong><p>Set Pool base URL, API key và default model/combo trên server.</p></div>
          ) : !pool.available ? (
            <div className={styles.empty}><strong>Chưa gọi được 9Router gateway.</strong><p>Kiểm tra deployment, API key và Endpoint → Require API Key trên Pool dashboard.</p></div>
          ) : pool.models.length === 0 ? (
            <div className={styles.empty}><strong>Pool chưa expose model nào.</strong><p>Kết nối provider hoặc tạo Combo trong 9Router rồi refresh.</p></div>
          ) : (
            <div className={styles.modelList}>
              {pool.models.map((model) => (
                <div className={styles.modelRow} key={model}>
                  <code>{model}</code>
                  <span>{model === pool.defaultModel ? "Auto default" : "Available"}</span>
                </div>
              ))}
            </div>
          )}
        </section>

        <aside className={styles.panel}>
          <span className={styles.kicker}>Setup boundary</span>
          <h3>Account nằm ở Pool, không nằm trong CodeLocal DB.</h3>
          <ol className={styles.steps}>
            <li><b>1</b><span>Mở Pool dashboard và connect Codex / Claude / Gemini / API providers.</span></li>
            <li><b>2</b><span>Tạo Combo làm route mặc định và cấu hình fallback/quota trong 9Router.</span></li>
            <li><b>3</b><span>Bật Endpoint → Require API Key, sau đó tạo key riêng cho CodeLocal.</span></li>
            <li><b>4</b><span>Đặt key + model/combo mặc định vào CodeLocal Cloud rồi dùng model Auto.</span></li>
          </ol>
          <div className={styles.boundary}>
            <strong>CodeLocal stores</strong>
            <span>Pool URL</span><span>Pool API key</span><span>Default model/combo ID</span>
          </div>
          <div className={styles.boundary}>
            <strong>9Router stores</strong>
            <span>OAuth sessions</span><span>Provider API keys</span><span>Quota / fallback state</span>
          </div>
        </aside>
      </div>
    </div>
  );
}
