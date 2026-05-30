import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  plugins: [react()],
  resolve: { alias: { "@": path.resolve(__dirname, "./src") } },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:8080",
      "/users": "http://localhost:8080",
    },
  },
  build: {
    // Output directly into the Go server's embed directory so `go build`
    // picks it up via go:embed. Falls back to local "dist" when the Go tree
    // isn't available (e.g. in a docker build of the UI alone).
    outDir: process.env.UI_OUT_DIR ?? "../web-api/internal/interfaces/http/embedded",
    emptyOutDir: true,
  },
});
