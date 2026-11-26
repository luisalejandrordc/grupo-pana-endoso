import { afterEach, describe, expect, it, vi } from "vitest";
import { CustomerApiClient } from "./client";

afterEach(() => vi.unstubAllGlobals());

function stub(handler: (path: string, init?: RequestInit) => Response) {
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) =>
      handler(String(input), init),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const ok = (body: unknown) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });

describe("CustomerApiClient", () => {
  it("renews and ends a session on the customers' own routes, never the staff's", async () => {
    const fetchMock = stub((path) =>
      path.endsWith("/auth/customer/refresh")
        ? ok({ accessToken: "fresh" })
        : new Response(null, { status: 204 }),
    );
    const client = new CustomerApiClient({ baseUrl: "/api/v1/" });

    await client.refreshSession();
    expect(client.getToken()).toBe("fresh");
    await client.logout();
    expect(client.getToken()).toBeNull();

    const paths = fetchMock.mock.calls.map(([url]) => String(url));
    expect(paths).toEqual([
      "/api/v1/auth/customer/refresh",
      "/api/v1/auth/customer/logout",
    ]);
    const [, refreshInit] = fetchMock.mock.calls[0];
    expect(new Headers(refreshInit?.headers).get("X-Requested-With")).toBe(
      "EndosoFlow",
    );
  });

  it("answers null when there is no session to renew", async () => {
    stub(() => new Response("{}", { status: 401 }));
    const client = new CustomerApiClient({ baseUrl: "/api/v1" });
    expect(await client.refreshSession()).toBeNull();
  });

  it("renews an expired token once through the customer route, then repeats the call", async () => {
    let salesCalls = 0;
    const fetchMock = stub((path) => {
      if (path.endsWith("/auth/customer/refresh"))
        return ok({ accessToken: "second" });
      salesCalls += 1;
      return salesCalls === 1
        ? new Response(
            JSON.stringify({
              error: { code: "ACCESS_TOKEN_INVALID", message: "x" },
            }),
            { status: 401 },
          )
        : ok({ items: [] });
    });
    const client = new CustomerApiClient({ baseUrl: "/api/v1" });
    client.setToken("first");

    await expect(client.listSales()).resolves.toEqual({ items: [] });
    expect(
      fetchMock.mock.calls
        .map(([url]) => String(url))
        .filter((u) => u.includes("/auth/")),
    ).toEqual(["/api/v1/auth/customer/refresh"]);
  });
});
