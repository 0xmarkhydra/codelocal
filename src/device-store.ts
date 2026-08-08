import { createHash, randomBytes, randomUUID, timingSafeEqual } from "node:crypto";
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

function hashSecret(secret: string) {
  return createHash("sha256").update(secret).digest("hex");
}

function safeEqualHex(a: string, b: string) {
  const aa = Buffer.from(a, "hex");
  const bb = Buffer.from(b, "hex");
  return aa.length === bb.length && timingSafeEqual(aa, bb);
}

export class DeviceStore {
  private loaded = false;
  private state: StoreShape = { devices: [], pairings: [] };

  constructor(private file = process.env.CODELOCAL_SERVER_STATE_FILE ?? path.join(DEFAULT_STATE_DIR, "server-devices.json")) {}

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
    const credentialSecret = randomBytes(32).toString("base64url");
    this.state.devices.push({
      credentialId,
      deviceId: pairing.deviceId,
      deviceName: pairing.deviceName,
      secretHash: hashSecret(credentialSecret),
      createdAt: Date.now(),
      lastSeenAt: Date.now(),
    });
    await this.save();
    return { credentialId, credentialSecret, deviceId: pairing.deviceId, deviceName: pairing.deviceName };
  }

  async authenticate(credentialId: string, credentialSecret: string) {
    await this.load();
    const device = this.state.devices.find((d) => d.credentialId === credentialId && !d.revokedAt);
    if (!device) return null;
    const ok = safeEqualHex(device.secretHash, hashSecret(credentialSecret));
    if (!ok) return null;
    device.lastSeenAt = Date.now();
    await this.save();
    return { ...device, secretHash: "[REDACTED]" };
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
    const credentialSecret = randomBytes(32).toString("base64url");
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
