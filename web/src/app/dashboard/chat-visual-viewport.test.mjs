import assert from "node:assert/strict";
import { chatViewportFrame } from "./chat-visual-viewport.ts";

assert.deepEqual(chatViewportFrame(null, 844), { height: 844, offsetTop: 0 });
assert.deepEqual(chatViewportFrame({ height: 512.4, offsetTop: 47.6 }, 844), { height: 512, offsetTop: 48 });
assert.deepEqual(chatViewportFrame({ height: 0, offsetTop: -12 }, 844), { height: 1, offsetTop: 0 });
