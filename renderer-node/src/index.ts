// Renderer entrypoint: consume pipeline.assemble.requested, render MP4, publish.
import { createServer } from "node:http";
import Redis from "ioredis";
import { Bus, Message } from "./redis.js";
import { ObjectStore } from "./minio.js";
import { load } from "./settings.js";
import { assemble, AssembleInput } from "./render.js";
import { log } from "./log.js";

let ready = false;

function startHealthServer(port: number, redisUrl: string) {
  const probe = new Redis(redisUrl, { lazyConnect: true, maxRetriesPerRequest: 1 });
  const tick = async () => {
    try {
      await probe.ping();
      ready = true;
    } catch {
      ready = false;
    }
  };
  void tick();
  setInterval(tick, 5000);

  const server = createServer((req, res) => {
    if (req.url === "/health") {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ status: "ok" }));
      return;
    }
    if (req.url === "/ready") {
      res.writeHead(ready ? 200 : 503, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ ready }));
      return;
    }
    res.writeHead(404);
    res.end();
  });
  server.listen(port, () => log.info("health server listening", { port }));
}

const REQ = "pipeline.assemble.requested";
const DONE = "pipeline.assemble.completed";
const FAIL = "pipeline.assemble.failed";

const cancelled = new Set<string>();

async function watchCancellations(bus: Bus) {
  await bus.consume("pipeline.run.cancelled", async (msg) => {
    const id = String(msg.run_id ?? "");
    if (id) cancelled.add(id);
  });
}

async function main() {
  const cfg = load();
  const healthPort = Number(process.env.HEALTH_PORT ?? 8082);
  startHealthServer(healthPort, cfg.redisUrl);
  const bus = new Bus(cfg.redisUrl, cfg.consumerGroup, cfg.consumerName);
  // Cancellation watcher uses its own consumer group — every replica needs to see every event.
  const cancelBus = new Bus(cfg.redisUrl, `cancel-watch-${process.pid}`, cfg.consumerName);
  const store = new ObjectStore(cfg.minio);
  log.info("renderer listening", { stream: REQ });
  watchCancellations(cancelBus).catch((e) => log.error("cancel watcher died", { err: String(e) }));
  await bus.consume(REQ, async (msg: Message) => {
    const ctx = log.bind({
      run_id: String(msg.run_id ?? ""),
      step_index: Number(msg.step_index ?? 0),
      step_type: "assemble",
    });
    if (cancelled.has(String(msg.run_id ?? ""))) {
      ctx.info("skipping cancelled run");
      return;
    }
    const start = Date.now();
    try {
      const input: AssembleInput = {
        runId: String(msg.run_id ?? ""),
        stepIndex: Number(msg.step_index ?? 0),
        panels: (msg.panels as any[]).map((p) => ({
          index: Number(p.index ?? 0),
          objectKey: String(p.object_key ?? ""),
          durationMs: Number(p.duration_ms ?? 2500),
          transition: String(p.transition ?? "crossfade"),
          transition_in: p.transition_in,
          effects: p.effects,
          caption: p.caption,
        })),
        outputKey: String(msg.output_key ?? `runs/${msg.run_id}/${msg.step_index}/video.mp4`),
        width: Number(msg.width ?? 1080),
        height: Number(msg.height ?? 1080),
        fps: Number(msg.fps ?? 30),
        audioKey: msg.audio_key ? String(msg.audio_key) : undefined,
        musicKey: msg.music_key ? String(msg.music_key) : undefined,
        ambientKey: msg.ambient_key ? String(msg.ambient_key) : undefined,
        sfxKeys: (msg.sfx_keys && typeof msg.sfx_keys === "object")
          ? Object.fromEntries(Object.entries(msg.sfx_keys as Record<string, unknown>)
              .map(([k, v]) => [k, String(v)]))
          : undefined,
      };
      const result = await assemble(input, store);
      const durationMs = Date.now() - start;
      await bus.publish(DONE, {
        run_id: input.runId,
        step_index: input.stepIndex,
        object_key: result.outputKey,
        bucket: store.bucket,
        bytes: result.bytes,
        cost: {
          provider: "local",
          model: "remotion",
          units: durationMs / 1000,
          unit_label: "seconds",
          unit_cost_usd: 0,
          total_cost_usd: 0,
        },
        duration_ms: durationMs,
      });
      ctx.info("assemble done", { duration_ms: durationMs, bytes: result.bytes });
    } catch (e: any) {
      ctx.error("assemble failed", { err: String(e?.message ?? e) });
      await bus.publish(FAIL, {
        run_id: String(msg.run_id ?? ""),
        step_index: Number(msg.step_index ?? 0),
        error: String(e?.message ?? e),
      });
    }
  });
}

main().catch((e) => {
  console.error("fatal", e);
  process.exit(1);
});
