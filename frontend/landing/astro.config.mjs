import { defineConfig } from "astro/config";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  output: "static",
  server: { port: 4321, host: true },
  vite: {
    plugins: [tailwindcss()],
    server: {
      allowedHosts: [".trycloudflare.com"],
    },
  },
});
