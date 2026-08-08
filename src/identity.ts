import os from "node:os";
import path from "node:path";
import { unlink } from "node:fs/promises";
import { DEFAULT_STATE_DIR, readJsonFile, writeJsonAtomic } from "./state.js";

export type LocalDeviceCredential = {
  credentialId: string;
  credentialSecret: string;
  deviceId: string;
  deviceName: string;
  serverUrl: string;
  createdAt: number;
};

const CREDENTIAL_FILE = process.env.CODELOCAL_CREDENTIAL_FILE ?? path.join(DEFAULT_STATE_DIR, "device-credential.json");

export async function loadLocalCredential(serverUrl?: string) {
  const value = await readJsonFile<LocalDeviceCredential | null>(CREDENTIAL_FILE, null);
  if (!value) return null;
  if (serverUrl && value.serverUrl.replace(/\/$/, "") !== serverUrl.replace(/\/$/, "")) return null;
  return value;
}

export async function saveLocalCredential(value: Omit<LocalDeviceCredential, "createdAt"> & { createdAt?: number }) {
  const credential: LocalDeviceCredential = {
    ...value,
    createdAt: value.createdAt ?? Date.now(),
  };
  await writeJsonAtomic(CREDENTIAL_FILE, credential);
  return credential;
}

export async function deleteLocalCredential() {
  await unlink(CREDENTIAL_FILE).catch(() => undefined);
}

export function defaultDeviceIdentity() {
  return {
    deviceId: process.env.CODELOCAL_DEVICE_ID ?? os.hostname(),
    deviceName: process.env.CODELOCAL_DEVICE_NAME ?? os.hostname(),
  };
}
