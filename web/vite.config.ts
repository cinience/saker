import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { viteSingleFile } from "vite-plugin-singlefile";
import path from "path";

export default defineConfig({
  plugins: [react(), viteSingleFile()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
      "node-fetch": path.resolve(__dirname, "src/lib/node-fetch-shim.ts"),
    },
  },
  build: {
    outDir: "out",
    emptyOutDir: true,
  },
  server: {
    port: 10111,
    proxy: {
      "/api": "http://localhost:10110",
      "/ws": { target: "http://localhost:10110", ws: true },
    },
  },
});
