import { promises as fs } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "..");
const staging = path.join(root, ".release", "npm");
const stagingDist = path.join(staging, "dist");

const runtimeFiles = [
  "approval.js",
  "audit.js",
  "chat-approval.js",
  "cli-saas.js",
  "cli.js",
  "client-entry-v2.js",
  "client-v2.js",
  "cloud-client-sync.js",
  "context-engine.js",
  "editing-engine.js",
  "identity.js",
  "log.js",
  "lsp.js",
  "mcp-cloud-sync.js",
  "mcp-hub.js",
  "native-watcher.js",
  "process-manager.js",
  "protocol.js",
  "runtime-daemon.js",
  "sandbox.js",
  "security-policy.js",
  "semantic-router.js",
  "semantic.js",
  "state.js",
  "terminal-history.js",
  "verification.js",
  "workspace-index.js",
  "workspace-registry.js",
];

await fs.rm(staging, { recursive: true, force: true });
await fs.mkdir(stagingDist, { recursive: true });

for (const filename of runtimeFiles) {
  const source = path.join(root, "dist", filename);
  const target = path.join(stagingDist, filename);
  await fs.copyFile(source, target);
}

const manifest = {
  name: "codelocal",
  version: "1.5.0-dev.1",
  description: "CodeLocal local code intelligence and execution runtime for ChatGPT.",
  license: "UNLICENSED",
  type: "module",
  bin: { codelocal: "dist/cli-saas.js" },
  files: ["dist/*.js", "README.md"],
  publishConfig: { access: "public" },
  dependencies: {
    "@modelcontextprotocol/sdk": "^1.18.2",
    "@parcel/watcher": "^2.6.0",
    chokidar: "^4.0.3",
    ignore: "^7.0.5",
    typescript: "^5.9.2",
    ws: "^8.18.3"
  },
  optionalDependencies: {
    "node-pty": "^1.0.0"
  },
  engines: { node: ">=20" }
};

const readme = `# CodeLocal\n\nLocal code intelligence and execution runtime for ChatGPT.\n\n## Install\n\n\`\`\`bash\nnpm i -g codelocal\n\`\`\`\n\n## Run\n\n\`\`\`bash\ncodelocal\n\`\`\`\n\nCodeLocal starts the machine runtime in your terminal and connects it to ChatGPT.\n\nTo authorize the current project once:\n\n\`\`\`bash\ncodelocal .\n\`\`\`\n\nThis npm package contains compiled runtime files only. The CodeLocal source repository and cloud backend are not distributed in this package.\n\nCopyright © CodeLocal. All rights reserved.\n`;

await fs.writeFile(path.join(staging, "package.json"), `${JSON.stringify(manifest, null, 2)}\n`, { mode: 0o600 });
await fs.writeFile(path.join(staging, "README.md"), readme, { mode: 0o600 });

console.log(`Prepared npm release: ${staging}`);
console.log(`Package: ${manifest.name}@${manifest.version}`);
console.log(`Runtime files: ${runtimeFiles.length}`);
