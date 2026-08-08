import { randomUUID } from "node:crypto";
import { Pool, type PoolClient } from "pg";
import { createClient, type RedisClientType } from "redis";

export type CloudUser = {
  id: string;
  email: string;
  passwordHash: string;
  passwordSalt: string;
  createdAt: number;
};

export type CloudDevice = {
  credentialId: string;
  userId: string;
  deviceId: string;
  deviceName: string;
  secretHash: string;
  createdAt: number;
  lastSeenAt: number;
  revokedAt?: number;
};

export type CloudPairing = {
  pairingId: string;
  code: string;
  deviceId: string;
  deviceName: string;
  userId?: string;
  createdAt: number;
  expiresAt: number;
  approvedAt?: number;
  claimedAt?: number;
};

export type CloudWorkspace = {
  userId: string;
  deviceId: string;
  workspaceId: string;
  workspaceName: string;
  projectRoot?: string;
  protocolVersion?: number;
  capabilities?: Record<string, unknown>;
  createdAt: number;
  lastSeenAt: number;
};

export type CloudMcpInstallation = {
  id: string;
  userId: string;
  name: string;
  enabled: boolean;
  scope: "global" | "workspace";
  workspaceId?: string;
  transport: "stdio" | "http";
  config: Record<string, unknown>;
  requiredSecrets: string[];
  createdAt: number;
  updatedAt: number;
};

export type OAuthClientRecord = {
  clientId: string;
  redirectUris: string[];
  clientName?: string;
  createdAt: number;
};

export type OAuthCodeRecord = {
  code: string;
  userId: string;
  clientId: string;
  redirectUri: string;
  codeChallenge: string;
  resource: string;
  scope: string;
  expiresAt: number;
};

function rowNumber(value: unknown) {
  return value == null ? undefined : Number(value);
}

function normalizeEmail(value: string) {
  return value.trim().toLowerCase();
}

export class CloudStore {
  private pool: Pool | null = null;
  private redis: RedisClientType | null = null;
  private initialized = false;
  private initPromise: Promise<void> | null = null;

  get enabled() {
    return !!process.env.DATABASE_URL && !!process.env.REDIS_URL;
  }

  async init() {
    if (this.initialized) return;
    if (this.initPromise) return this.initPromise;
    this.initPromise = this.initialize();
    try {
      await this.initPromise;
      this.initialized = true;
    } finally {
      this.initPromise = null;
    }
  }

  private async initialize() {
    const databaseUrl = process.env.DATABASE_URL;
    const redisUrl = process.env.REDIS_URL;
    if (!databaseUrl || !redisUrl) throw new Error("CodeLocal SaaS requires DATABASE_URL and REDIS_URL.");

    this.pool = new Pool({
      connectionString: databaseUrl,
      max: Number(process.env.CODELOCAL_DB_POOL_SIZE ?? 10),
      ssl: databaseUrl.includes("railway.internal") ? false : undefined,
    });
    await this.pool.query("select 1");
    await this.ensureSchema();

    this.redis = createClient({ url: redisUrl });
    this.redis.on("error", (error) => console.error(JSON.stringify({ ts: new Date().toISOString(), level: "error", event: "redis.error", error: String(error) })));
    await this.redis.connect();
    await this.redis.ping();
  }

  async close() {
    if (this.redis?.isOpen) await this.redis.quit().catch(() => undefined);
    await this.pool?.end().catch(() => undefined);
    this.redis = null;
    this.pool = null;
    this.initialized = false;
  }

  private db() {
    if (!this.pool) throw new Error("CloudStore is not initialized.");
    return this.pool;
  }

  private cache() {
    if (!this.redis) throw new Error("CloudStore Redis is not initialized.");
    return this.redis;
  }

  private async ensureSchema() {
    const db = this.db();
    await db.query(`
      CREATE TABLE IF NOT EXISTS codelocal_users (
        id TEXT PRIMARY KEY,
        email TEXT NOT NULL UNIQUE,
        password_hash TEXT NOT NULL,
        password_salt TEXT NOT NULL,
        created_at BIGINT NOT NULL
      );

      CREATE TABLE IF NOT EXISTS codelocal_devices (
        credential_id TEXT PRIMARY KEY,
        user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
        device_id TEXT NOT NULL,
        device_name TEXT NOT NULL,
        secret_hash TEXT NOT NULL,
        created_at BIGINT NOT NULL,
        last_seen_at BIGINT NOT NULL,
        revoked_at BIGINT,
        UNIQUE(user_id, device_id)
      );

      CREATE TABLE IF NOT EXISTS codelocal_pairings (
        pairing_id TEXT PRIMARY KEY,
        code TEXT NOT NULL,
        device_id TEXT NOT NULL,
        device_name TEXT NOT NULL,
        user_id TEXT REFERENCES codelocal_users(id) ON DELETE CASCADE,
        created_at BIGINT NOT NULL,
        expires_at BIGINT NOT NULL,
        approved_at BIGINT,
        claimed_at BIGINT
      );

      CREATE TABLE IF NOT EXISTS codelocal_workspaces (
        user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
        device_id TEXT NOT NULL,
        workspace_id TEXT NOT NULL,
        workspace_name TEXT NOT NULL,
        project_root TEXT,
        protocol_version INTEGER,
        capabilities JSONB NOT NULL DEFAULT '{}'::jsonb,
        created_at BIGINT NOT NULL,
        last_seen_at BIGINT NOT NULL,
        PRIMARY KEY(user_id, device_id, workspace_id)
      );

      CREATE TABLE IF NOT EXISTS codelocal_oauth_clients (
        client_id TEXT PRIMARY KEY,
        redirect_uris JSONB NOT NULL,
        client_name TEXT,
        created_at BIGINT NOT NULL
      );

      CREATE TABLE IF NOT EXISTS codelocal_oauth_codes (
        code TEXT PRIMARY KEY,
        user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
        client_id TEXT NOT NULL REFERENCES codelocal_oauth_clients(client_id) ON DELETE CASCADE,
        redirect_uri TEXT NOT NULL,
        code_challenge TEXT NOT NULL,
        resource TEXT NOT NULL,
        scope TEXT NOT NULL,
        expires_at BIGINT NOT NULL
      );

      CREATE TABLE IF NOT EXISTS codelocal_mcp_installations (
        id TEXT PRIMARY KEY,
        user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
        name TEXT NOT NULL,
        enabled BOOLEAN NOT NULL DEFAULT TRUE,
        scope TEXT NOT NULL CHECK (scope IN ('global', 'workspace')),
        workspace_id TEXT,
        transport TEXT NOT NULL CHECK (transport IN ('stdio', 'http')),
        config JSONB NOT NULL DEFAULT '{}'::jsonb,
        required_secrets JSONB NOT NULL DEFAULT '[]'::jsonb,
        created_at BIGINT NOT NULL,
        updated_at BIGINT NOT NULL,
        UNIQUE(user_id, name, scope, workspace_id)
      );

      CREATE TABLE IF NOT EXISTS codelocal_permissions (
        id TEXT PRIMARY KEY,
        user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
        workspace_id TEXT,
        capability TEXT NOT NULL,
        decision TEXT NOT NULL CHECK (decision IN ('allow', 'ask', 'deny')),
        updated_at BIGINT NOT NULL,
        UNIQUE(user_id, workspace_id, capability)
      );

      CREATE TABLE IF NOT EXISTS codelocal_audit_logs (
        id TEXT PRIMARY KEY,
        user_id TEXT REFERENCES codelocal_users(id) ON DELETE SET NULL,
        event TEXT NOT NULL,
        device_id TEXT,
        workspace_id TEXT,
        detail JSONB NOT NULL DEFAULT '{}'::jsonb,
        created_at BIGINT NOT NULL
      );

      CREATE INDEX IF NOT EXISTS idx_codelocal_devices_user ON codelocal_devices(user_id);
      CREATE INDEX IF NOT EXISTS idx_codelocal_workspaces_user ON codelocal_workspaces(user_id, last_seen_at DESC);
      CREATE INDEX IF NOT EXISTS idx_codelocal_mcp_user ON codelocal_mcp_installations(user_id, updated_at DESC);
      CREATE INDEX IF NOT EXISTS idx_codelocal_audit_user ON codelocal_audit_logs(user_id, created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_codelocal_pairings_expires ON codelocal_pairings(expires_at);
    `);
  }

  async createUser(email: string, passwordHash: string, passwordSalt: string) {
    const id = randomUUID();
    const createdAt = Date.now();
    const normalized = normalizeEmail(email);
    try {
      await this.db().query(
        "INSERT INTO codelocal_users(id,email,password_hash,password_salt,created_at) VALUES($1,$2,$3,$4,$5)",
        [id, normalized, passwordHash, passwordSalt, createdAt],
      );
    } catch (error: any) {
      if (error?.code === "23505") throw new Error("EMAIL_ALREADY_REGISTERED");
      throw error;
    }
    return { id, email: normalized, passwordHash, passwordSalt, createdAt } satisfies CloudUser;
  }

  async userByEmail(email: string): Promise<CloudUser | null> {
    const result = await this.db().query("SELECT * FROM codelocal_users WHERE email=$1", [normalizeEmail(email)]);
    const row = result.rows[0];
    if (!row) return null;
    return { id: row.id, email: row.email, passwordHash: row.password_hash, passwordSalt: row.password_salt, createdAt: Number(row.created_at) };
  }

  async userById(id: string): Promise<CloudUser | null> {
    const result = await this.db().query("SELECT * FROM codelocal_users WHERE id=$1", [id]);
    const row = result.rows[0];
    if (!row) return null;
    return { id: row.id, email: row.email, passwordHash: row.password_hash, passwordSalt: row.password_salt, createdAt: Number(row.created_at) };
  }

  async createSession(userId: string, csrf: string, ttlSeconds = 30 * 24 * 60 * 60) {
    const sessionId = randomUUID() + randomUUID().replaceAll("-", "");
    await this.cache().set(`codelocal:session:${sessionId}`, JSON.stringify({ userId, csrf }), { EX: ttlSeconds });
    return sessionId;
  }

  async readSession(sessionId: string): Promise<{ userId: string; csrf: string } | null> {
    if (!sessionId) return null;
    const raw = await this.cache().get(`codelocal:session:${sessionId}`);
    if (!raw) return null;
    try {
      const value = JSON.parse(raw) as { userId?: string; csrf?: string };
      return value.userId && value.csrf ? { userId: value.userId, csrf: value.csrf } : null;
    } catch {
      return null;
    }
  }

  async deleteSession(sessionId: string) {
    if (sessionId) await this.cache().del(`codelocal:session:${sessionId}`);
  }

  async createPairing(deviceId: string, deviceName: string, ttlMs = 10 * 60_000): Promise<CloudPairing> {
    const pairing: CloudPairing = {
      pairingId: randomUUID(),
      code: String(Math.floor(100000 + Math.random() * 900000)),
      deviceId,
      deviceName,
      createdAt: Date.now(),
      expiresAt: Date.now() + ttlMs,
    };
    await this.db().query(
      "INSERT INTO codelocal_pairings(pairing_id,code,device_id,device_name,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6)",
      [pairing.pairingId, pairing.code, pairing.deviceId, pairing.deviceName, pairing.createdAt, pairing.expiresAt],
    );
    return pairing;
  }

  async getPairing(pairingId: string): Promise<CloudPairing | null> {
    const result = await this.db().query("SELECT * FROM codelocal_pairings WHERE pairing_id=$1", [pairingId]);
    return this.mapPairing(result.rows[0]);
  }

  async approvePairing(pairingId: string, code: string, userId: string): Promise<CloudPairing | null> {
    const now = Date.now();
    const result = await this.db().query(
      `UPDATE codelocal_pairings SET user_id=$3, approved_at=$4
       WHERE pairing_id=$1 AND code=$2 AND expires_at>$4 AND claimed_at IS NULL
       RETURNING *`,
      [pairingId, code, userId, now],
    );
    return this.mapPairing(result.rows[0]);
  }

  async claimPairing(pairingId: string, code: string, credentialId: string, secretHash: string): Promise<CloudDevice | null> {
    const client = await this.db().connect();
    try {
      await client.query("BEGIN");
      const pairingResult = await client.query(
        `SELECT * FROM codelocal_pairings
         WHERE pairing_id=$1 AND code=$2 AND expires_at>$3 AND approved_at IS NOT NULL AND claimed_at IS NULL
         FOR UPDATE`,
        [pairingId, code, Date.now()],
      );
      const pairing = this.mapPairing(pairingResult.rows[0]);
      if (!pairing?.userId) { await client.query("ROLLBACK"); return null; }
      const now = Date.now();
      await client.query("UPDATE codelocal_pairings SET claimed_at=$2 WHERE pairing_id=$1", [pairingId, now]);
      await client.query(
        `INSERT INTO codelocal_devices(credential_id,user_id,device_id,device_name,secret_hash,created_at,last_seen_at)
         VALUES($1,$2,$3,$4,$5,$6,$6)
         ON CONFLICT(user_id,device_id) DO UPDATE SET credential_id=EXCLUDED.credential_id, device_name=EXCLUDED.device_name,
           secret_hash=EXCLUDED.secret_hash, created_at=EXCLUDED.created_at, last_seen_at=EXCLUDED.last_seen_at, revoked_at=NULL`,
        [credentialId, pairing.userId, pairing.deviceId, pairing.deviceName, secretHash, now],
      );
      await client.query("COMMIT");
      return { credentialId, userId: pairing.userId, deviceId: pairing.deviceId, deviceName: pairing.deviceName, secretHash, createdAt: now, lastSeenAt: now };
    } catch (error) {
      await client.query("ROLLBACK").catch(() => undefined);
      throw error;
    } finally {
      client.release();
    }
  }

  async authenticateDevice(credentialId: string, secretHash: string): Promise<CloudDevice | null> {
    const result = await this.db().query(
      "SELECT * FROM codelocal_devices WHERE credential_id=$1 AND secret_hash=$2 AND revoked_at IS NULL",
      [credentialId, secretHash],
    );
    const row = result.rows[0];
    if (!row) return null;
    const now = Date.now();
    await this.db().query("UPDATE codelocal_devices SET last_seen_at=$2 WHERE credential_id=$1", [credentialId, now]);
    return this.mapDevice({ ...row, last_seen_at: now });
  }

  async listDevices(userId: string): Promise<CloudDevice[]> {
    const result = await this.db().query("SELECT * FROM codelocal_devices WHERE user_id=$1 ORDER BY last_seen_at DESC", [userId]);
    return result.rows.map((row) => this.mapDevice(row)!);
  }

  async revokeDevice(userId: string, credentialId: string) {
    const result = await this.db().query(
      "UPDATE codelocal_devices SET revoked_at=$3 WHERE user_id=$1 AND credential_id=$2 AND revoked_at IS NULL",
      [userId, credentialId, Date.now()],
    );
    return result.rowCount === 1;
  }

  async renameDevice(userId: string, credentialId: string, deviceName: string) {
    const result = await this.db().query(
      "UPDATE codelocal_devices SET device_name=$3 WHERE user_id=$1 AND credential_id=$2 AND revoked_at IS NULL",
      [userId, credentialId, deviceName],
    );
    return result.rowCount === 1;
  }

  async rotateDevice(userId: string, credentialId: string, secretHash: string) {
    const result = await this.db().query(
      "UPDATE codelocal_devices SET secret_hash=$3 WHERE user_id=$1 AND credential_id=$2 AND revoked_at IS NULL RETURNING *",
      [userId, credentialId, secretHash],
    );
    return this.mapDevice(result.rows[0]);
  }

  async upsertWorkspace(input: Omit<CloudWorkspace, "createdAt" | "lastSeenAt">) {
    const now = Date.now();
    await this.db().query(
      `INSERT INTO codelocal_workspaces(user_id,device_id,workspace_id,workspace_name,project_root,protocol_version,capabilities,created_at,last_seen_at)
       VALUES($1,$2,$3,$4,$5,$6,$7,$8,$8)
       ON CONFLICT(user_id,device_id,workspace_id) DO UPDATE SET workspace_name=EXCLUDED.workspace_name,
         project_root=EXCLUDED.project_root, protocol_version=EXCLUDED.protocol_version, capabilities=EXCLUDED.capabilities, last_seen_at=EXCLUDED.last_seen_at`,
      [input.userId, input.deviceId, input.workspaceId, input.workspaceName, input.projectRoot ?? null, input.protocolVersion ?? null, JSON.stringify(input.capabilities ?? {}), now],
    );
    await this.setPresence(input.userId, input.deviceId, input.workspaceId);
  }

  async touchWorkspace(userId: string, deviceId: string, workspaceId: string) {
    const now = Date.now();
    await this.db().query("UPDATE codelocal_workspaces SET last_seen_at=$4 WHERE user_id=$1 AND device_id=$2 AND workspace_id=$3", [userId, deviceId, workspaceId, now]);
    await this.setPresence(userId, deviceId, workspaceId);
  }

  async setPresence(userId: string, deviceId: string, workspaceId: string, ttlSeconds = 90) {
    await this.cache().set(`codelocal:presence:${userId}:${deviceId}:${workspaceId}`, String(Date.now()), { EX: ttlSeconds });
  }

  async clearPresence(userId: string, deviceId: string, workspaceId: string) {
    await this.cache().del(`codelocal:presence:${userId}:${deviceId}:${workspaceId}`);
  }

  async isWorkspaceOnline(userId: string, deviceId: string, workspaceId: string) {
    return !!(await this.cache().exists(`codelocal:presence:${userId}:${deviceId}:${workspaceId}`));
  }

  async listWorkspaces(userId: string): Promise<Array<CloudWorkspace & { online: boolean }>> {
    const result = await this.db().query("SELECT * FROM codelocal_workspaces WHERE user_id=$1 ORDER BY last_seen_at DESC", [userId]);
    return Promise.all(result.rows.map(async (row) => {
      const workspace: CloudWorkspace = {
        userId: row.user_id,
        deviceId: row.device_id,
        workspaceId: row.workspace_id,
        workspaceName: row.workspace_name,
        projectRoot: row.project_root ?? undefined,
        protocolVersion: row.protocol_version ?? undefined,
        capabilities: row.capabilities ?? {},
        createdAt: Number(row.created_at),
        lastSeenAt: Number(row.last_seen_at),
      };
      return { ...workspace, online: await this.isWorkspaceOnline(userId, workspace.deviceId, workspace.workspaceId) };
    }));
  }

  async createOAuthClient(client: Omit<OAuthClientRecord, "createdAt">) {
    const createdAt = Date.now();
    await this.db().query(
      "INSERT INTO codelocal_oauth_clients(client_id,redirect_uris,client_name,created_at) VALUES($1,$2,$3,$4)",
      [client.clientId, JSON.stringify(client.redirectUris), client.clientName ?? null, createdAt],
    );
    return { ...client, createdAt };
  }

  async oauthClient(clientId: string): Promise<OAuthClientRecord | null> {
    const result = await this.db().query("SELECT * FROM codelocal_oauth_clients WHERE client_id=$1", [clientId]);
    const row = result.rows[0];
    return row ? { clientId: row.client_id, redirectUris: row.redirect_uris ?? [], clientName: row.client_name ?? undefined, createdAt: Number(row.created_at) } : null;
  }

  async putOAuthCode(code: OAuthCodeRecord) {
    await this.db().query(
      `INSERT INTO codelocal_oauth_codes(code,user_id,client_id,redirect_uri,code_challenge,resource,scope,expires_at)
       VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
      [code.code, code.userId, code.clientId, code.redirectUri, code.codeChallenge, code.resource, code.scope, code.expiresAt],
    );
  }

  async consumeOAuthCode(code: string): Promise<OAuthCodeRecord | null> {
    const client = await this.db().connect();
    try {
      await client.query("BEGIN");
      const result = await client.query("DELETE FROM codelocal_oauth_codes WHERE code=$1 RETURNING *", [code]);
      await client.query("COMMIT");
      const row = result.rows[0];
      return row ? {
        code: row.code,
        userId: row.user_id,
        clientId: row.client_id,
        redirectUri: row.redirect_uri,
        codeChallenge: row.code_challenge,
        resource: row.resource,
        scope: row.scope,
        expiresAt: Number(row.expires_at),
      } : null;
    } catch (error) {
      await client.query("ROLLBACK").catch(() => undefined);
      throw error;
    } finally {
      client.release();
    }
  }

  async listMcpInstallations(userId: string, workspaceId?: string): Promise<CloudMcpInstallation[]> {
    const result = await this.db().query(
      `SELECT * FROM codelocal_mcp_installations
       WHERE user_id=$1 AND (scope='global' OR (scope='workspace' AND workspace_id=$2))
       ORDER BY updated_at DESC`,
      [userId, workspaceId ?? ""],
    );
    return result.rows.map((row) => this.mapMcp(row));
  }

  async allMcpInstallations(userId: string) {
    const result = await this.db().query("SELECT * FROM codelocal_mcp_installations WHERE user_id=$1 ORDER BY updated_at DESC", [userId]);
    return result.rows.map((row) => this.mapMcp(row));
  }

  async upsertMcpInstallation(input: Omit<CloudMcpInstallation, "id" | "createdAt" | "updatedAt">) {
    const existing = await this.db().query(
      `SELECT id,created_at FROM codelocal_mcp_installations
       WHERE user_id=$1 AND name=$2 AND scope=$3 AND workspace_id IS NOT DISTINCT FROM $4`,
      [input.userId, input.name, input.scope, input.workspaceId ?? null],
    );
    const id = existing.rows[0]?.id ?? randomUUID();
    const createdAt = existing.rows[0] ? Number(existing.rows[0].created_at) : Date.now();
    const updatedAt = Date.now();
    await this.db().query(
      `INSERT INTO codelocal_mcp_installations(id,user_id,name,enabled,scope,workspace_id,transport,config,required_secrets,created_at,updated_at)
       VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
       ON CONFLICT(id) DO UPDATE SET enabled=EXCLUDED.enabled, transport=EXCLUDED.transport, config=EXCLUDED.config,
         required_secrets=EXCLUDED.required_secrets, updated_at=EXCLUDED.updated_at`,
      [id, input.userId, input.name, input.enabled, input.scope, input.workspaceId ?? null, input.transport, JSON.stringify(input.config), JSON.stringify(input.requiredSecrets), createdAt, updatedAt],
    );
    return { id, ...input, createdAt, updatedAt };
  }

  async removeMcpInstallation(userId: string, id: string) {
    const result = await this.db().query("DELETE FROM codelocal_mcp_installations WHERE user_id=$1 AND id=$2", [userId, id]);
    return result.rowCount === 1;
  }

  async audit(userId: string | undefined, event: string, detail: Record<string, unknown> = {}, deviceId?: string, workspaceId?: string) {
    await this.db().query(
      "INSERT INTO codelocal_audit_logs(id,user_id,event,device_id,workspace_id,detail,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)",
      [randomUUID(), userId ?? null, event, deviceId ?? null, workspaceId ?? null, JSON.stringify(detail), Date.now()],
    );
  }

  async recentAudit(userId: string, limit = 50) {
    const result = await this.db().query(
      "SELECT id,event,device_id,workspace_id,detail,created_at FROM codelocal_audit_logs WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2",
      [userId, Math.max(1, Math.min(limit, 200))],
    );
    return result.rows.map((row) => ({ id: row.id, event: row.event, deviceId: row.device_id, workspaceId: row.workspace_id, detail: row.detail ?? {}, createdAt: Number(row.created_at) }));
  }

  private mapPairing(row: any): CloudPairing | null {
    if (!row) return null;
    return {
      pairingId: row.pairing_id,
      code: row.code,
      deviceId: row.device_id,
      deviceName: row.device_name,
      userId: row.user_id ?? undefined,
      createdAt: Number(row.created_at),
      expiresAt: Number(row.expires_at),
      approvedAt: rowNumber(row.approved_at),
      claimedAt: rowNumber(row.claimed_at),
    };
  }

  private mapDevice(row: any): CloudDevice | null {
    if (!row) return null;
    return {
      credentialId: row.credential_id,
      userId: row.user_id,
      deviceId: row.device_id,
      deviceName: row.device_name,
      secretHash: row.secret_hash,
      createdAt: Number(row.created_at),
      lastSeenAt: Number(row.last_seen_at),
      revokedAt: rowNumber(row.revoked_at),
    };
  }

  private mapMcp(row: any): CloudMcpInstallation {
    return {
      id: row.id,
      userId: row.user_id,
      name: row.name,
      enabled: !!row.enabled,
      scope: row.scope,
      workspaceId: row.workspace_id ?? undefined,
      transport: row.transport,
      config: row.config ?? {},
      requiredSecrets: Array.isArray(row.required_secrets) ? row.required_secrets : [],
      createdAt: Number(row.created_at),
      updatedAt: Number(row.updated_at),
    };
  }
}

export const cloudStore = new CloudStore();
