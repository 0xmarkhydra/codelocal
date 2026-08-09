import assert from "node:assert/strict";
import test from "node:test";
import { hashPassword, verifyPassword } from "../saas-auth.js";

test("SaaS passwords are salted and verified without storing plaintext", async () => {
  const password = "correct horse battery staple";
  const first = await hashPassword(password);
  const second = await hashPassword(password);
  assert.notEqual(first.salt, second.salt);
  assert.notEqual(first.hash, second.hash);
  assert.equal(await verifyPassword(password, first.salt, first.hash), true);
  assert.equal(await verifyPassword("wrong password", first.salt, first.hash), false);
  assert.equal(first.hash.includes(password), false);
});
