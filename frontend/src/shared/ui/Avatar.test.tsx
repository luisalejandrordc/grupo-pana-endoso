import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Avatar } from "./Avatar";

afterEach(() => vi.unstubAllGlobals());

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

function setup(handler: (path: string) => Response) {
  const calls: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const path = new URL(String(input), "http://localhost").pathname.replace(
        "/api/v1",
        "",
      );
      calls.push(path);
      return handler(path);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  const view = (props: Parameters<typeof Avatar>[0]) => (
    <QueryClientProvider client={client}>
      <Avatar {...props} />
    </QueryClientProvider>
  );
  return { calls, view };
}

describe("Avatar", () => {
  it("shows initials and asks for nothing when the person has no picture", () => {
    const { calls, view } = setup(() => json({}, 500));
    render(view({ id: "e1", name: "Ana Rojas", hasPhoto: false }));
    expect(screen.getByText("AR")).toBeInTheDocument();
    expect(calls).toEqual([]);
  });

  it("loads the picture from its signed address when there is one", async () => {
    const { calls, view } = setup(() =>
      json({
        url: "https://files.test/e1.png?sig=1",
        expiresAt: "2099-01-01T00:00:00Z",
        mimeType: "image/png",
      }),
    );
    render(view({ id: "e1", name: "Ana Rojas", hasPhoto: true }));
    const img = await screen.findByRole("img", { name: "Ana Rojas" });
    expect(img).toHaveAttribute("src", "https://files.test/e1.png?sig=1");
    expect(calls).toEqual(["/employees/e1/photo"]);
  });

  it("keeps showing initials if the picture cannot be fetched", async () => {
    const { calls, view } = setup(() =>
      json(
        { error: { code: "PHOTO_NOT_FOUND", message: "No hay foto." } },
        404,
      ),
    );
    render(view({ id: "e1", name: "Ana Rojas", hasPhoto: true }));
    await waitFor(() => expect(calls.length).toBe(1));
    expect(screen.getByText("AR")).toBeInTheDocument();
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
  });

  it("falls back to initials if the image itself fails to load", async () => {
    const { view } = setup(() =>
      json({
        url: "https://files.test/dead.png",
        expiresAt: "2099-01-01T00:00:00Z",
        mimeType: "image/png",
      }),
    );
    render(view({ id: "e1", name: "Ana Rojas", hasPhoto: true }));
    fireEvent.error(await screen.findByRole("img", { name: "Ana Rojas" }));
    expect(await screen.findByText("AR")).toBeInTheDocument();
  });
});
