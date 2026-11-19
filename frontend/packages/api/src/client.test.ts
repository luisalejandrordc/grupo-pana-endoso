import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient, ApiError } from "./client";

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

function stub(...answers: (Response | (() => Response))[]) {
  const fetchMock = vi.fn(async () => {
    const next = answers.shift();
    if (!next) throw new Error("unexpected extra request");
    return typeof next === "function" ? next() : next;
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const call = (fetchMock: ReturnType<typeof stub>, n: number) => {
  const [url, init] = fetchMock.mock.calls[n] as unknown as [
    string,
    RequestInit,
  ];
  return { url, init, headers: new Headers(init.headers) };
};

afterEach(() => vi.unstubAllGlobals());

describe("ApiClient", () => {
  it("calls the base URL (without a trailing slash), with cookies and a JSON body", async () => {
    const fetchMock = stub(json({ ok: true }));
    const client = new ApiClient({
      baseUrl: "https://api.example.com/api/v1/",
    });
    await client.request("/things", { method: "POST", body: '{"a":1}' });
    const { url, init, headers } = call(fetchMock, 0);
    expect(url).toBe("https://api.example.com/api/v1/things");
    expect(init.credentials).toBe("include");
    expect(headers.get("Content-Type")).toBe("application/json");
  });

  it("lets the browser set the content type of a file upload", async () => {
    const fetchMock = stub(json({}));
    const client = new ApiClient({ baseUrl: "/api/v1" });
    const form = new FormData();
    form.append("file", new Blob(["x"]), "a.png");
    await client.request("/me/photo", { method: "PUT", body: form });
    expect(call(fetchMock, 0).headers.has("Content-Type")).toBe(false);
  });

  it("sends the access token once it has one", async () => {
    const fetchMock = stub(json({}), json({}));
    const client = new ApiClient({ baseUrl: "/api/v1" });
    await client.request("/a");
    client.setToken("abc");
    await client.request("/b");
    expect(call(fetchMock, 0).headers.has("Authorization")).toBe(false);
    expect(call(fetchMock, 1).headers.get("Authorization")).toBe("Bearer abc");
  });

  it("turns a refused request into an ApiError with the server's own words", async () => {
    stub(
      json(
        {
          error: {
            code: "VALIDATION_FAILED",
            message: "Revisa los datos.",
            fields: [
              { field: "email", code: "INVALID", message: "No válido." },
            ],
            requestId: "req-1",
          },
        },
        422,
      ),
    );
    const client = new ApiClient({ baseUrl: "/api/v1" });
    await expect(client.request("/x")).rejects.toMatchObject({
      name: "ApiError",
      message: "Revisa los datos.",
      code: "VALIDATION_FAILED",
      status: 422,
      requestId: "req-1",
      fields: [{ field: "email", code: "INVALID", message: "No válido." }],
    });
  });

  it("reads a 204 as nothing, and an answer that is not JSON as a generic failure", async () => {
    stub(
      new Response(null, { status: 204 }),
      new Response("<html>", { status: 502 }),
    );
    const client = new ApiClient({ baseUrl: "/api/v1" });
    await expect(client.request("/a")).resolves.toBeUndefined();
    const failure = await client.request("/b").catch((e: unknown) => e);
    expect(failure).toBeInstanceOf(ApiError);
    expect(failure).toMatchObject({ code: "REQUEST_FAILED", status: 502 });
  });

  describe("when the access token expires", () => {
    it("renews it once and repeats the call with the new token", async () => {
      const fetchMock = stub(
        json({}, 401),
        json({ accessToken: "new" }),
        json({ data: 1 }),
      );
      const client = new ApiClient({ baseUrl: "/api/v1" });
      client.setToken("old");
      await expect(client.request("/sales")).resolves.toEqual({ data: 1 });
      expect(call(fetchMock, 1).url).toBe("/api/v1/auth/refresh");
      expect(call(fetchMock, 1).headers.get("X-Requested-With")).toBe(
        "EndosoFlow",
      );
      expect(call(fetchMock, 2).headers.get("Authorization")).toBe(
        "Bearer new",
      );
      expect(client.getToken()).toBe("new");
    });

    it("gives up, forgets the token and tells the app when it cannot be renewed", async () => {
      stub(
        json(
          { error: { code: "UNAUTHENTICATED", message: "Sesión vencida." } },
          401,
        ),
        json({}, 401),
      );
      const client = new ApiClient({ baseUrl: "/api/v1" });
      const expired = vi.fn();
      client.setUnauthorizedHandler(expired);
      client.setToken("old");
      await expect(client.request("/sales")).rejects.toMatchObject({
        status: 401,
      });
      expect(expired).toHaveBeenCalledOnce();
      expect(client.getToken()).toBeNull();
    });

    it("does not try to renew for the auth endpoints themselves, nor when asked not to", async () => {
      const fetchMock = stub(json({}, 401), json({}, 401));
      const client = new ApiClient({ baseUrl: "/api/v1" });
      client.setToken("t");
      await client.request("/auth/logout").catch(() => undefined);
      await client.request("/x", { skipRefresh: true }).catch(() => undefined);
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });

    it("does not try to renew when there was no session to begin with", async () => {
      const fetchMock = stub(json({}, 401));
      const client = new ApiClient({ baseUrl: "/api/v1" });
      await client.request("/me").catch(() => undefined);
      expect(fetchMock).toHaveBeenCalledOnce();
    });
  });

  it("forgets the token on logout even if the server fails", async () => {
    stub(json({}, 500));
    const client = new ApiClient({ baseUrl: "/api/v1" });
    client.setToken("t");
    await client.logout().catch(() => undefined);
    expect(client.getToken()).toBeNull();
  });
});
