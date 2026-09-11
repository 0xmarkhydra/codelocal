import { createClient, type RedisClientType } from "redis";

export type WorkspaceActivation = {
  workspaceId: string;
  requestedAt: number;
  requestId: string;
};

export type WorkspaceRevocation = {
  workspaceId: string;
  requestedAt: number;
  requestId: string;
};

function safePart(value: string) {
  return encodeURIComponent(value).replaceAll("%", "_");
}

const SIGNAL_CHANNEL = "codelocal:runtime:signal";

export class RuntimeActivationStore {
  private redis: RedisClientType | null = null;
  private subscriber: RedisClientType | null = null;
  private initPromise: Promise<void> | null = null;
  private waiters = new Map<string, Set<() => void>>();
  private closing = false;
  private subscriberRetryAt = 0;

  private async init() {
    if (this.redis?.isOpen && (this.subscriber?.isOpen || Date.now() < this.subscriberRetryAt)) return;
    if (this.initPromise) return this.initPromise;
    this.initPromise = (async () => {
      const url = process.env.REDIS_URL;
      if (!url) throw new Error("Runtime activation requires REDIS_URL.");

      if (!this.redis?.isOpen) {
        const client = createClient({ url });
        client.on("error", (error) => console.error(JSON.stringify({ ts: new Date().toISOString(), level: "error", event: "runtime.redis_error", error: String(error) })));
        await client.connect();
        this.redis = client as RedisClientType;
      }

      if (!this.subscriber?.isOpen) {
        const subscriber = this.redis!.duplicate();
        subscriber.on("error", (error) => console.error(JSON.stringify({ ts: new Date().toISOString(), level: "error", event: "runtime.redis_subscriber_error", error: String(error) })));
        try {
          await subscriber.connect();
          await subscriber.subscribe(SIGNAL_CHANNEL, (message) => {
            try {
              const parsed = JSON.parse(message) as { userId?: string; deviceId?: string };
              if (!parsed.userId || !parsed.deviceId) return;
              this.notifyWaiters(parsed.userId, parsed.deviceId);
            } catch {}
          });
          this.subscriber = subscriber as RedisClientType;
          this.subscriberRetryAt = 0;
        } catch (error) {
          await subscriber.quit().catch(() => subscriber.disconnect().catch(() => undefined));
          this.subscriberRetryAt = Date.now() + 30_000;
          console.error(JSON.stringify({ ts: new Date().toISOString(), level: "warn", event: "runtime.redis_subscriber_unavailable", retryInMs: 30_000, error: String(error) }));
          // Queue consumption remains durable; long polls fall back to their timeout.
          this.subscriber = null;
        }
      }
    })();
    try { await this.initPromise; } finally { this.initPromise = null; }
  }

  private activationKey(userId: string, deviceId: string) {
    return `codelocal:runtime:activation:${safePart(userId)}:${safePart(deviceId)}`;
  }

  private revocationKey(userId: string, deviceId: string) {
    return `codelocal:runtime:revocation:${safePart(userId)}:${safePart(deviceId)}`;
  }

  private presenceKey(userId: string, deviceId: string) {
    return `codelocal:runtime:presence:${safePart(userId)}:${safePart(deviceId)}`;
  }

  private authorizedKey(userId: string, deviceId: string) {
    return `codelocal:runtime:authorized:${safePart(userId)}:${safePart(deviceId)}`;
  }

  private waiterKey(userId: string, deviceId: string) {
    return `${userId}\u0000${deviceId}`;
  }

  private notifyWaiters(userId: string, deviceId: string) {
    const key = this.waiterKey(userId, deviceId);
    const waiters = this.waiters.get(key);
    if (!waiters?.size) return;
    this.waiters.delete(key);
    for (const notify of waiters) notify();
  }

  private createSignalWaiter(userId: string, deviceId: string, timeoutMs: number) {
    const key = this.waiterKey(userId, deviceId);
    let settled = false;
    let resolvePromise: (signaled: boolean) => void = () => undefined;
    const finish = (signaled: boolean) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      const set = this.waiters.get(key);
      set?.delete(notify);
      if (set && !set.size) this.waiters.delete(key);
      resolvePromise(signaled);
    };
    const notify = () => finish(true);
    const promise = new Promise<boolean>((resolve) => { resolvePromise = resolve; });
    const set = this.waiters.get(key) ?? new Set<() => void>();
    set.add(notify);
    this.waiters.set(key, set);
    const timer = setTimeout(() => finish(false), Math.max(1, timeoutMs));
    timer.unref?.();
    return { promise, cancel: () => finish(false) };
  }

  private async publishSignal(userId: string, deviceId: string) {
    await this.redis!.publish(SIGNAL_CHANNEL, JSON.stringify({ userId, deviceId }));
  }

  async heartbeat(userId: string, deviceId: string, workspaceIds: readonly unknown[] = [], ttlSeconds = 20) {
    await this.init();
    const ids = [...new Set(workspaceIds.map((value) => String(value)).filter(Boolean))].slice(0, 500);
    await Promise.all([
      this.redis!.set(this.presenceKey(userId, deviceId), String(Date.now()), { EX: ttlSeconds }),
      this.redis!.set(this.authorizedKey(userId, deviceId), JSON.stringify(ids), { EX: ttlSeconds }),
    ]);
  }

  async isOnline(userId: string, deviceId: string) {
    await this.init();
    return !!(await this.redis!.exists(this.presenceKey(userId, deviceId)));
  }

  async authorizedIds(userId: string, deviceId: string) {
    await this.init();
    const raw = await this.redis!.get(this.authorizedKey(userId, deviceId));
    if (!raw) return null;
    try {
      const value = JSON.parse(raw);
      return Array.isArray(value) ? value.map(String) : [];
    } catch {
      return [];
    }
  }

  async isAuthorized(userId: string, deviceId: string, workspaceId: string) {
    const ids = await this.authorizedIds(userId, deviceId);
    return ids == null ? false : ids.includes(workspaceId);
  }

  async request(userId: string, deviceId: string, activation: WorkspaceActivation, ttlSeconds = 60) {
    await this.init();
    const key = this.activationKey(userId, deviceId);
    await this.redis!.rPush(key, JSON.stringify(activation));
    await this.redis!.expire(key, ttlSeconds);
    await this.publishSignal(userId, deviceId).catch(() => undefined);
  }

  async consume(userId: string, deviceId: string): Promise<WorkspaceActivation | null> {
    await this.init();
    const raw = await this.redis!.lPop(this.activationKey(userId, deviceId));
    if (!raw) return null;
    try {
      const value = JSON.parse(raw) as WorkspaceActivation;
      if (!value.workspaceId || !value.requestId) return null;
      return value;
    } catch {
      return null;
    }
  }

  async requestRevocation(userId: string, deviceId: string, revocation: WorkspaceRevocation, ttlSeconds = 120) {
    await this.init();
    const key = this.revocationKey(userId, deviceId);
    await this.redis!.rPush(key, JSON.stringify(revocation));
    await this.redis!.expire(key, ttlSeconds);
    await this.publishSignal(userId, deviceId).catch(() => undefined);
  }

  async consumeRevocation(userId: string, deviceId: string): Promise<WorkspaceRevocation | null> {
    await this.init();
    const raw = await this.redis!.lPop(this.revocationKey(userId, deviceId));
    if (!raw) return null;
    try {
      const value = JSON.parse(raw) as WorkspaceRevocation;
      if (!value.workspaceId || !value.requestId) return null;
      return value;
    } catch {
      return null;
    }
  }

  async waitForNext(userId: string, deviceId: string, timeoutMs = 0) {
    await this.init();
    const deadline = Date.now() + Math.max(0, Math.min(timeoutMs, 30_000));

    while (true) {
      if (this.closing) return { activation: null, revocation: null };
      const remainingMs = Math.max(0, deadline - Date.now());
      const waiter = remainingMs > 0 ? this.createSignalWaiter(userId, deviceId, remainingMs) : null;
      const revocation = await this.consumeRevocation(userId, deviceId);
      if (revocation) {
        waiter?.cancel();
        return { activation: null, revocation };
      }
      const activation = await this.consume(userId, deviceId);
      if (activation) {
        waiter?.cancel();
        return { activation, revocation: null };
      }
      if (!waiter) return { activation: null, revocation: null };
      if (!(await waiter.promise)) continue;
    }
  }

  async clearPresence(userId: string, deviceId: string) {
    await this.init();
    await this.redis!.del([
      this.presenceKey(userId, deviceId),
      this.authorizedKey(userId, deviceId),
      this.activationKey(userId, deviceId),
      this.revocationKey(userId, deviceId),
    ]);
  }

  async close() {
    this.closing = true;
    const pendingWaiters = [...this.waiters.values()].flatMap((waiters) => [...waiters]);
    this.waiters.clear();
    for (const notify of pendingWaiters) notify();
    if (this.subscriber?.isOpen) await this.subscriber.quit().catch(() => undefined);
    if (this.redis?.isOpen) await this.redis.quit().catch(() => undefined);
    this.subscriber = null;
    this.redis = null;
  }
}

export const runtimeActivationStore = new RuntimeActivationStore();
