import { createHash, randomBytes, randomUUID } from "node:crypto";
import { cloudStore } from "./cloud-store.js";

function hashSecret(secret: string) {
  return createHash("sha256").update(secret).digest("hex");
}

export class DeviceStore {
  async startPairing(deviceId: string, deviceName: string, ttlMs = 10 * 60_000) {
    return cloudStore.createPairing(deviceId, deviceName, ttlMs);
  }

  async getPairing(pairingId: string) {
    return cloudStore.getPairing(pairingId);
  }

  async approvePairing(pairingId: string, code: string, userId: string) {
    return cloudStore.approvePairing(pairingId, code, userId);
  }

  async claimPairing(pairingId: string, code: string) {
    const credentialId = `cld_${randomUUID()}`;
    const credentialSecret = randomBytes(40).toString("base64url");
    const device = await cloudStore.claimPairing(pairingId, code, credentialId, hashSecret(credentialSecret));
    if (!device) return null;
    return {
      credentialId,
      credentialSecret,
      userId: device.userId,
      deviceId: device.deviceId,
      deviceName: device.deviceName,
    };
  }

  async authenticate(credentialId: string, credentialSecret: string) {
    const device = await cloudStore.authenticateDevice(credentialId, hashSecret(credentialSecret));
    if (!device) return null;
    return { ...device, secretHash: "[REDACTED]" };
  }

  async listDevices(userId: string) {
    const devices = await cloudStore.listDevices(userId);
    return devices.map(({ secretHash: _secretHash, ...device }) => device);
  }

  async revoke(userId: string, credentialId: string) {
    return cloudStore.revokeDevice(userId, credentialId);
  }

  async rotate(userId: string, credentialId: string) {
    const credentialSecret = randomBytes(40).toString("base64url");
    const device = await cloudStore.rotateDevice(userId, credentialId, hashSecret(credentialSecret));
    if (!device) return null;
    return { credentialId, credentialSecret };
  }

  async rename(userId: string, credentialId: string, deviceName: string) {
    return cloudStore.renameDevice(userId, credentialId, deviceName);
  }
}
