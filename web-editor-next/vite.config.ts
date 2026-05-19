import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { viteSingleFile } from "vite-plugin-singlefile";
import wasm from "vite-plugin-wasm";
import path from "path";

export default defineConfig({
  plugins: [react(), tailwindcss(), wasm(), viteSingleFile()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "src") },
  },
  build: {
    outDir: "out",
    emptyOutDir: true,
    target: "esnext",
  },
  server: {
    port: 10112,
    proxy: {
      "/api": "http://localhost:10110",
      "/ws": { target: "http://localhost:10110", ws: true },
    },
  },
  worker: {
    format: "es",
  },
});
