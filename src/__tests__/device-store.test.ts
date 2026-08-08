import test from "node:test";
import assert from "node:assert/strict";
import { promises as fs } from "node:fs";
import os from "node:os";
import path from "node:path";
import { DeviceStore } from "../device-store.js";

test("pair approve claim authenticate rotate revoke lifecycle", async () => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "codelocal-device-"));
  try {
    const store = new DeviceStore(path.join(root, "devices.json"));
    const pairing = await store.startPairing("device-a", "Mac A", 60_000);
    assert.equal((await store.approvePairing(pairing.pairingId, "wrong")), null);
    assert.ok(await store.approvePairing(pairing.pairingId, pairing.code));
    const credential = await store.claimPairing(pairing.pairingId, pairing.code);
    assert.ok(credential);
    assert.ok(await store.authenticate(credential!.credentialId, credential!.credentialSecret));
    const rotated = await store.rotate(credential!.credentialId);
    assert.ok(rotated);
    assert.equal(await store.authenticate(credential!.credentialId, credential!.credentialSecret), null);
    assert.ok(await store.authenticate(rotated!.credentialId, rotated!.credentialSecret));
    assert.equal(await store.revoke(credential!.credentialId), true);
    assert.equal(await store.authenticate(rotated!.credentialId, rotated!.credentialSecret), null);
  } finally {
    await fs.rm(root, { recursive: true, force: true });
  }
});
