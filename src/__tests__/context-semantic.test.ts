import test from "node:test";
import assert from "node:assert/strict";
import { promises as fs } from "node:fs";
import os from "node:os";
import path from "node:path";
import { SemanticRouter } from "../semantic-router.js";
import { ProjectContextEngine } from "../context-engine.js";

test("builds compact project map and resolves TypeScript symbols", async () => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "codelocal-context-"));
  try {
    await fs.mkdir(path.join(root, "src"), { recursive: true });
    await fs.writeFile(path.join(root, "package.json"), JSON.stringify({ scripts: { build: "tsc", test: "node --test" }, dependencies: { express: "1.0.0" }, devDependencies: { typescript: "5.9.2" } }), "utf8");
    await fs.writeFile(path.join(root, "tsconfig.json"), JSON.stringify({ compilerOptions: { target: "ES2022", module: "ESNext" }, include: ["src/**/*.ts"] }), "utf8");
    await fs.writeFile(path.join(root, "src", "service.ts"), "export class PaymentService { charge() { return 1; } }\nexport function usePayment() { return new PaymentService().charge(); }\n", "utf8");
    const semantic = new SemanticRouter(root);
    const context = new ProjectContextEngine(root, semantic);
    const map = await context.map();
    assert.ok(map.languages.includes("typescript/javascript"));
    assert.ok(map.frameworks.includes("express"));
    assert.ok(map.buildCommands.some((x) => x.includes("build")));
    const symbols = await semantic.workspaceSymbols("PaymentService", 20);
    assert.ok(symbols.some((x: any) => x.name === "PaymentService"));
    const relevant = await context.relevant("fix PaymentService charge", 20);
    assert.ok(relevant.relevantPaths.includes("src/service.ts"));
    await semantic.shutdown();
  } finally {
    await fs.rm(root, { recursive: true, force: true });
  }
});
