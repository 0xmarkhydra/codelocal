import assert from "node:assert/strict";
import { threadBelongsToWorkspace, workspaceIdentityKey } from "./chat-workspace-key.ts";

const workspace = { deviceId: "device-1", workspaceId: "workspace-1" };

assert.equal(workspaceIdentityKey(workspace), "device-1::workspace-1");
assert.equal(threadBelongsToWorkspace({ workspaceKey: "device-1::workspace-1" }, workspace), true);
assert.equal(threadBelongsToWorkspace({ workspaceKey: "device-1::workspace-2" }, workspace), false);
assert.equal(threadBelongsToWorkspace({}, workspace), false);
