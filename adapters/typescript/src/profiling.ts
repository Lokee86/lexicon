import * as fs from "node:fs";
import * as path from "node:path";

interface ProfilePhase {
  name: string;
  duration_ns: number;
}

class AdapterProfiler {
  private readonly outputPath = process.env.LEXICON_ADAPTER_PROFILE ?? "";
  private readonly phases: ProfilePhase[] = [];
  private readonly counts: Record<string, number> = {};

  measure<T>(name: string, operation: () => T): T {
    if (!this.outputPath) return operation();
    const started = process.hrtime.bigint();
    try {
      return operation();
    } finally {
      this.phases.push({ name, duration_ns: Number(process.hrtime.bigint() - started) });
    }
  }

  set(name: string, value: number): void {
    if (this.outputPath) this.counts[name] = value;
  }

  write(): void {
    if (!this.outputPath) return;
    fs.mkdirSync(path.dirname(this.outputPath), { recursive: true });
    fs.writeFileSync(
      this.outputPath,
      `${JSON.stringify({ version: 1, phases: this.phases, counts: this.counts }, null, 2)}\n`,
      "utf8",
    );
  }
}

export const adapterProfile = new AdapterProfiler();
