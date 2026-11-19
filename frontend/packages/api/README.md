# @endosoflow/api

What every site needs to talk to the Go backend. It depends on nothing of ours (TanStack Query as a peer, for `createQueryClient`).

| Export                     | What it is                                                                                              |
| -------------------------- | ------------------------------------------------------------------------------------------------------- |
| `ApiClient`, `ApiError`    | The HTTP core: keeps the access token, sends cookies, renews an expired token once and repeats the call |
| `Schemas`, `Operations`    | Types generated from `backend/api/openapi.yaml` (`Schemas["Sale"]`, `Operations["listSales"]`)          |
| `queryString`              | Builds `?a=1&b=2` from an object, leaving out empty values                                              |
| `errorText`, `fieldErrors` | The message to show for any failure, and the per-field errors of a refused form                         |
| `createQueryClient`        | One cache configuration for every site (a 4xx is final, a 5xx is retried once)                          |

## A site extends the core with its own login

```ts
import { ApiClient } from "@endosoflow/api";

class CustomerApiClient extends ApiClient {
  async login(input: { documentNumber: string; password: string }) {
    const session = await this.request<{ accessToken: string }>(
      "/auth/customer/login",
      {
        method: "POST",
        body: JSON.stringify(input),
        skipRefresh: true,
      },
    );
    this.setToken(session.accessToken);
    return session;
  }
}

export const api = new CustomerApiClient({
  baseUrl: import.meta.env.VITE_API_BASE_URL || "/api/v1",
});
```

`baseUrl` should stay `/api/v1` in production when the site is on a platform that proxies it (see `docs/DEPLOY.md`): that keeps the refresh cookie first-party.

## When the backend contract changes

```bash
npm run api:types          # regenerate src/schema.d.ts (from the workspace root)
npm run api:types:check    # fails if the committed file is out of date: good for CI
```

Then `npm run typecheck`: any call that no longer matches the contract fails to compile in the sites.
