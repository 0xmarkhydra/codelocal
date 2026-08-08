import { mkdir, open, readFile, rename, unlink, chmod, appendFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import os from "node:os";
import { randomUUID } from "node:crypto";

export const DEFAULT_STATE_DIR = process.env.CODELOCAL_STATE_DIR ?? join(os.homedir(), ".codelocal");

export async function ensurePrivateDir(dir = DEFAULT_STATE_DIR) {
  await mkdir(dir, { recursive: true, mode: 0o700 });
  if (process.platform !== "win32") await chmod(dir, 0o700).catch(() => undefined);
  return dir;
}

export async function readJsonFile<T>(file: string, fallback: T): Promise<T> {
  try {
    return JSON.parse(await readFile(file, "utf8")) as T;
  } catch {
    return fallback;
  }
}

export async function writeJsonAtomic(file: string, value: unknown) {
  const parent = dirname(file);
  await ensurePrivateDir(parent);
  const temp = join(parent, `.${randomUUID()}.tmp`);
  let created = false;
  try {
    const handle = await open(temp, "wx", 0o600);
    created = true;
    try {
      if (process.platform !== "win32") await handle.chmod(0o600).catch(() => undefined);
      await handle.writeFile(`${JSON.stringify(value, null, 2)}\n`, "utf8");
      await handle.sync();
    } finally {
      await handle.close();
    }
    await rename(temp, file);
    created = false;
    if (process.platform !== "win32") await chmod(file, 0o600).catch(() => undefined);
  } finally {
    if (created) await unlink(temp).catch(() => undefined);
  }
}

export async function appendPrivateJsonl(file: string, value: unknown) {
  await ensurePrivateDir(dirname(file));
  await appendFile(file, `${JSON.stringify(value)}\n`, { encoding: "utf8", mode: 0o600 });
  if (process.platform !== "win32") await chmod(file, 0o600).catch(() => undefined);
}

export type IdempotencyJournalEntry = {
  key: string;
  operation: string;
  startedAt: number;
  completedAt?: number;
  status: "started" | "completed" | "failed";
  result?: unknown;
  error?: string;
};

export class IdempotencyJournal {
  private map = new Map<string, IdempotencyJournalEntry>();
  private loaded = false;
  constructor(private file = join(DEFAULT_STATE_DIR, "journal.json"), private maxEntries = 1000) {}

  private async load() {
    if (this.loaded) return;
    const entries = await readJsonFile<IdempotencyJournalEntry[]>(this.file, []);
    for (const entry of entries) this.map.set(entry.key, entry);
    this.loaded = true;
  }

  async get(key: string) {
    await this.load();
    return this.map.get(key) ?? null;
  }

  async start(key: string, operation: string) {
    await this.load();
    const existing = this.map.get(key);
    if (existing) return existing;
    const entry: IdempotencyJournalEntry = { key, operation, startedAt: Date.now(), status: "started" };
    this.map.set(key, entry);
    await this.flush();
    return entry;
  }

  async complete(key: string, result: unknown) {
    await this.load();
    const entry = this.map.get(key);
    if (!entry) return;
    entry.status = "completed";
    entry.completedAt = Date.now();
    entry.result = result;
    delete entry.error;
    await this.flush();
  }

  async fail(key: string, error: string) {
    await this.load();
    const entry = this.map.get(key);
    if (!entry) return;
    entry.status = "failed";
    entry.completedAt = Date.now();
    entry.error = error;
    await this.flush();
  }

  private async flush() {
    const values = [...this.map.values()].sort((a, b) => b.startedAt - a.startedAt).slice(0, this.maxEntries);
    this.map = new Map(values.map((x) => [x.key, x]));
    await writeJsonAtomic(this.file, values);
  }
}
