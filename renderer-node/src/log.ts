// Tiny JSON logger emitting the same field vocabulary as Go (platform/logger)
// and Python (workers-py/src/worker/log_fields.py).

type Fields = Record<string, unknown>;

const base: Fields = { service: "renderer-node", worker: "assemble" };

function emit(level: string, msg: string, fields: Fields = {}): void {
  const line = JSON.stringify({
    time: new Date().toISOString(),
    level,
    msg,
    ...base,
    ...fields,
  });
  if (level === "ERROR") console.error(line);
  else console.log(line);
}

export const log = {
  info: (msg: string, fields?: Fields) => emit("INFO", msg, fields),
  warn: (msg: string, fields?: Fields) => emit("WARN", msg, fields),
  error: (msg: string, fields?: Fields) => emit("ERROR", msg, fields),
  bind: (extra: Fields) => ({
    info: (msg: string, fields?: Fields) => emit("INFO", msg, { ...extra, ...fields }),
    warn: (msg: string, fields?: Fields) => emit("WARN", msg, { ...extra, ...fields }),
    error: (msg: string, fields?: Fields) => emit("ERROR", msg, { ...extra, ...fields }),
  }),
};
