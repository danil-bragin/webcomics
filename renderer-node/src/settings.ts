export type Settings = {
  redisUrl: string;
  consumerGroup: string;
  consumerName: string;
  minio: {
    endpoint: string;
    accessKey: string;
    secretKey: string;
    bucket: string;
    useSSL: boolean;
  };
};

export function load(): Settings {
  return {
    redisUrl: process.env.REDIS_URL ?? "redis://localhost:6380/0",
    consumerGroup: process.env.CONSUMER_GROUP ?? "pipeline-node-assemble",
    consumerName: process.env.CONSUMER_NAME ?? "renderer-1",
    minio: {
      endpoint: process.env.MINIO_ENDPOINT ?? "localhost:9000",
      accessKey: process.env.MINIO_ACCESS_KEY ?? "minioadmin",
      secretKey: process.env.MINIO_SECRET_KEY ?? "minioadmin",
      bucket: process.env.MINIO_BUCKET ?? "webcomics",
      useSSL: (process.env.MINIO_USE_SSL ?? "false") === "true",
    },
  };
}
