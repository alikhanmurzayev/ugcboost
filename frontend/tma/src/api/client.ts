import createClient from "openapi-fetch";

import type { paths } from "./generated/schema";
import { tmaInitDataMiddleware } from "./middleware";

function resolveApiUrl(): string {
  const runtime = window.__RUNTIME_CONFIG__?.apiUrl;
  if (runtime) return runtime;
  return "/api";
}

export const apiClient = createClient<paths>({ baseUrl: resolveApiUrl() });
apiClient.use(tmaInitDataMiddleware);
