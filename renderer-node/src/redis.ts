// Redis Streams XREADGROUP consumer + XADD publisher.
// Mirrors Watermill's `payload` field convention used by the Go side.

import Redis from "ioredis";

export type Message = Record<string, unknown>;

const PAYLOAD_FIELD = "payload";

export class Bus {
  private redis: Redis;
  constructor(url: string, private group: string, private consumer: string) {
    this.redis = new Redis(url);
  }

  async ensureGroup(stream: string): Promise<void> {
    try {
      await this.redis.xgroup("CREATE", stream, this.group, "0", "MKSTREAM");
    } catch (e: any) {
      if (!String(e?.message ?? e).includes("BUSYGROUP")) throw e;
    }
  }

  async consume(stream: string, handler: (m: Message) => Promise<void>): Promise<void> {
    await this.ensureGroup(stream);
    for (;;) {
      const resp = (await this.redis.xreadgroup(
        "GROUP",
        this.group,
        this.consumer,
        "COUNT",
        4,
        "BLOCK",
        5000,
        "STREAMS",
        stream,
        ">",
      )) as Array<[string, Array<[string, string[]]>]> | null;
      if (!resp) continue;
      for (const [, entries] of resp) {
        for (const [id, fields] of entries) {
          const idx = fields.indexOf(PAYLOAD_FIELD);
          if (idx < 0 || idx + 1 >= fields.length) {
            await this.redis.xack(stream, this.group, id);
            continue;
          }
          let msg: Message;
          try {
            msg = JSON.parse(fields[idx + 1]);
          } catch (e) {
            console.error("bad json", e);
            await this.redis.xack(stream, this.group, id);
            continue;
          }
          try {
            await handler(msg);
            await this.redis.xack(stream, this.group, id);
          } catch (e) {
            console.error("handler error", e);
            // leave un-ACKed for retry
          }
        }
      }
    }
  }

  async publish(stream: string, payload: Message): Promise<void> {
    const body = JSON.stringify(payload);
    await this.redis.xadd(stream, "*", PAYLOAD_FIELD, body);
  }

  async close(): Promise<void> {
    await this.redis.quit();
  }
}
