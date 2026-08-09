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

export class RuntimeActivationStore {
  private redis: RedisClientType | null = null;
  private initPromise: Promise<void> | null = null;

  private async init() {
    if (this.redis?.isOpen) return;
    if (this.initPromise) return this.initPromise;
    this.initPromise = (async () => {
      const url = process.env.REDIS_URL;
      if (!url) throw new Error("Runtime activation requires REDIS_URL.");
      const client = createClient({ url });
      client.on("error", (error) => console.error(JSON.stringify({ ts: new Date().toISOString(), level: "error", event: "runtime.redis_error", error: String(error) })));
      await client.connect();
      this.redis = client as RedisClientType;
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
    if (this.redis?.isOpen) await this.redis.quit().catch(() => undefined);
    this.redis = null;
  }
}

export const runtimeActivationStore = new RuntimeActivationStore();
