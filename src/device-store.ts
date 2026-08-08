import { createHash, createHmac, randomBytes, randomUUID, timingSafeEqual } from "node:crypto";
import path from "node:path";
import { DEFAULT_STATE_DIR, readJsonFile, writeJsonAtomic } from "./state.js";

export type DeviceIdentity = {
  credentialId: string;
  deviceId: string;
  deviceName: string;
  secretHash: string;
  createdAt: number;
  lastSeenAt: number;
  revokedAt?: number;
  capabilities?: Record<string, unknown>;
};

export type PairingRecord = {
  pairingId: string;
  code: string;
  deviceId: string;
  deviceName: string;
  createdAt: number;
  expiresAt: number;
  approvedAt?: number;
  claimedAt?: number;
};

type StoreShape = {
  devices: DeviceIdentity[];
  pairings: PairingRecord[];
};

type SignedCredentialPayload = {
  v: 1;
  credentialId: string;
  deviceId: string;
  deviceName: string;
  issuedAt: number;
};

function hashSecret(secret: string) {
  return createHash("sha256").update(secret).digest("hex");
}

function safeEqualHex(a: string, b: string) {
  const aa = Buffer.from(a, "hex");
  const bb = Buffer.from(b, "hex");
  return aa.length === bb.length && timingSafeEqual(aa, bb);
}

function safeEqualText(a: string, b: string) {
  const aa = Buffer.from(a);
  const bb = Buffer.from(b);
  return aa.length === bb.length && timingSafeEqual(aa, bb);
}

export class DeviceStore {
  private loaded = false;
  private state: StoreShape = { devices: [], pairings: [] };
  private signingSecret: string;

  constructor(
    private file = process.env.CODELOCAL_SERVER_STATE_FILE ?? path.join(DEFAULT_STATE_DIR, "server-devices.json"),
    signingSecret = process.env.CODELOCAL_DEVICE_AUTH_SECRET ?? process.env.MCP_AUTH_SECRET ?? "",
  ) {
    this.signingSecret = signingSecret;
  }

  private async load() {
    if (this.loaded) return;
    this.state = await readJsonFile<StoreShape>(this.file, { devices: [], pairings: [] });
    this.loaded = true;
    this.gcPairings();
  }

  private gcPairings() {
    const cutoff = Date.now() - 24 * 60 * 60_000;
    this.state.pairings = this.state.pairings.filter((p) => p.expiresAt > cutoff && !p.claimedAt);
  }

  private async save() {
    this.gcPairings();
    await writeJsonAtomic(this.file, this.state);
  }

  private signPayload(payload: SignedCredentialPayload) {
    if (!this.signingSecret) return null;
    const encoded = Buffer.from(JSON.stringify(payload), "utf8").toString("base64url");
    const signature = createHmac("sha256", this.signingSecret).update(encoded).digest("base64url");
    return `v1.${encoded}.${signature}`;
  }

  private verifySignedCredential(credentialId: string, credentialSecret: string) {
    if (!this.signingSecret || !credentialSecret.startsWith("v1.")) return null;
    const parts = credentialSecret.split(".");
    if (parts.length !== 3) return null;
    const [, encoded, signature] = parts;
    const expected = createHmac("sha256", this.signingSecret).update(encoded).digest("base64url");
    if (!safeEqualText(signature, expected)) return null;
    try {
      const payload = JSON.parse(Buffer.from(encoded, "base64url").toString("utf8")) as SignedCredentialPayload;
      if (payload.v !== 1 || payload.credentialId !== credentialId || !payload.deviceId || !payload.deviceName) return null;
      return payload;
    } catch {
      return null;
    }
  }

  private issueCredential(credentialId: string, deviceId: string, deviceName: string, issuedAt = Date.now()) {
    return this.signPayload({ v: 1, credentialId, deviceId, deviceName, issuedAt }) ?? randomBytes(32).toString("base64url");
  }

  async startPairing(deviceId: string, deviceName: string, ttlMs = 10 * 60_000) {
    await this.load();
    const pairing: PairingRecord = {
      pairingId: randomUUID(),
      code: String(Math.floor(100000 + Math.random() * 900000)),
      deviceId,
      deviceName,
      createdAt: Date.now(),
      expiresAt: Date.now() + ttlMs,
    };
    this.state.pairings.push(pairing);
    await this.save();
    return pairing;
  }

  async getPairing(pairingId: string) {
    await this.load();
    return this.state.pairings.find((p) => p.pairingId === pairingId) ?? null;
  }

  async approvePairing(pairingId: string, code: string) {
    await this.load();
    const pairing = this.state.pairings.find((p) => p.pairingId === pairingId);
    if (!pairing || pairing.expiresAt <= Date.now() || pairing.code !== code || pairing.claimedAt) return null;
    pairing.approvedAt = Date.now();
    await this.save();
    return pairing;
  }

  async claimPairing(pairingId: string, code: string) {
    await this.load();
    const pairing = this.state.pairings.find((p) => p.pairingId === pairingId);
    if (!pairing || pairing.expiresAt <= Date.now() || pairing.code !== code || !pairing.approvedAt || pairing.claimedAt) return null;
    pairing.claimedAt = Date.now();
    const credentialId = `dev_${randomUUID()}`;
    const createdAt = Date.now();
    const credentialSecret = this.issueCredential(credentialId, pairing.deviceId, pairing.deviceName, createdAt);
    this.state.devices.push({
      credentialId,
      deviceId: pairing.deviceId,
      deviceName: pairing.deviceName,
      secretHash: hashSecret(credentialSecret),
      createdAt,
      lastSeenAt: createdAt,
    });
    await this.save();
    return { credentialId, credentialSecret, deviceId: pairing.deviceId, deviceName: pairing.deviceName };
  }

  async authenticate(credentialId: string, credentialSecret: string) {
    await this.load();
    const device = this.state.devices.find((d) => d.credentialId === credentialId && !d.revokedAt);
    if (device && safeEqualHex(device.secretHash, hashSecret(credentialSecret))) {
      device.lastSeenAt = Date.now();
      await this.save();
      return { ...device, secretHash: "[REDACTED]" };
    }

    const signed = this.verifySignedCredential(credentialId, credentialSecret);
    if (!signed) return null;
    const revoked = this.state.devices.find((d) => d.credentialId === credentialId)?.revokedAt;
    if (revoked) return null;
    return {
      credentialId: signed.credentialId,
      deviceId: signed.deviceId,
      deviceName: signed.deviceName,
      createdAt: signed.issuedAt,
      lastSeenAt: Date.now(),
      secretHash: "[STATELESS]",
    };
  }

  async listDevices() {
    await this.load();
    return this.state.devices.map(({ secretHash: _secretHash, ...device }) => device);
  }

  async revoke(credentialId: string) {
    await this.load();
    const device = this.state.devices.find((d) => d.credentialId === credentialId);
    if (!device) return false;
    device.revokedAt = Date.now();
    await this.save();
    return true;
  }

  async rotate(credentialId: string) {
    await this.load();
    const device = this.state.devices.find((d) => d.credentialId === credentialId && !d.revokedAt);
    if (!device) return null;
    const credentialSecret = this.issueCredential(device.credentialId, device.deviceId, device.deviceName);
    device.secretHash = hashSecret(credentialSecret);
    await this.save();
    return { credentialId, credentialSecret };
  }

  async rename(credentialId: string, deviceName: string) {
    await this.load();
    const device = this.state.devices.find((d) => d.credentialId === credentialId && !d.revokedAt);
    if (!device) return false;
    device.deviceName = deviceName;
    await this.save();
    return true;
  }
}
