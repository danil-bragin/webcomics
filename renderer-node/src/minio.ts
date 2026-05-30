// MinIO/S3 helpers via @aws-sdk/client-s3 (S3-compat).

import {
  GetObjectCommand,
  PutObjectCommand,
  S3Client,
} from "@aws-sdk/client-s3";
import { Readable } from "node:stream";
import { Settings } from "./settings.js";

export class ObjectStore {
  private client: S3Client;
  get bucket(): string { return this.cfg.bucket; }
  constructor(private cfg: Settings["minio"]) {
    this.client = new S3Client({
      region: "us-east-1",
      endpoint: `${cfg.useSSL ? "https" : "http"}://${cfg.endpoint}`,
      forcePathStyle: true,
      credentials: { accessKeyId: cfg.accessKey, secretAccessKey: cfg.secretKey },
    });
  }

  async getBuffer(key: string): Promise<Buffer> {
    const r = await this.client.send(
      new GetObjectCommand({ Bucket: this.cfg.bucket, Key: key }),
    );
    return await streamToBuffer(r.Body as Readable);
  }

  async putBuffer(key: string, data: Buffer, contentType: string): Promise<void> {
    await this.client.send(
      new PutObjectCommand({
        Bucket: this.cfg.bucket,
        Key: key,
        Body: data,
        ContentType: contentType,
      }),
    );
  }
}

async function streamToBuffer(s: Readable): Promise<Buffer> {
  const chunks: Buffer[] = [];
  for await (const c of s) chunks.push(Buffer.isBuffer(c) ? c : Buffer.from(c));
  return Buffer.concat(chunks);
}
