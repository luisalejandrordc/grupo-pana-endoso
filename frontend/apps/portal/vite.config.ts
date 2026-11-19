import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv } from "vite";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".", "");
  const backend = env.VITE_API_PROXY_TARGET || "http://localhost:8080";
  return {
    plugins: [react()],
    server: {
      // The staff site owns 5173; the portal takes the next port so both can run together.
      port: 5174,
      strictPort: true,
      proxy: {
        "/api": { target: backend, changeOrigin: true },
        "/media": { target: backend, changeOrigin: true },
      },
    },
  };
});
