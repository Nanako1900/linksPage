import { describe, expect, it, vi } from "vitest";
import { qrPng } from "./qr";

function fakeCanvasDoc(ctx: Partial<CanvasRenderingContext2D> | null) {
  const canvas = {
    width: 0,
    height: 0,
    getContext: vi.fn().mockReturnValue(ctx),
    toDataURL: vi.fn().mockReturnValue("data:image/png;base64,AAA"),
  };
  const doc = { createElement: vi.fn().mockReturnValue(canvas) } as unknown as Document;
  return { canvas, doc };
}

describe("qrPng", () => {
  it("draws dark modules with a quiet zone and returns a PNG data URL", async () => {
    const fillRect = vi.fn();
    const { canvas, doc } = fakeCanvasDoc({ fillRect, fillStyle: "" });
    const encode = vi.fn().mockReturnValue({
      size: 2,
      data: [
        [true, false],
        [false, true],
      ],
    });
    const img = await qrPng("https://links.example.com/go/discord", 4, { load: async () => ({ encode }), doc });
    expect(encode).toHaveBeenCalledWith("https://links.example.com/go/discord", { ecc: "M", border: 0 });
    expect(img).toEqual({ src: "data:image/png;base64,AAA", size: 24 });
    expect(canvas.width).toBe(24);
    expect(canvas.toDataURL).toHaveBeenCalledWith("image/png");
    // background + two dark modules offset by the 2-module margin
    expect(fillRect.mock.calls).toEqual([
      [0, 0, 24, 24],
      [8, 8, 4, 4],
      [12, 12, 4, 4],
    ]);
  });

  it("encodes real data with uqr", async () => {
    const { doc } = fakeCanvasDoc({ fillRect: vi.fn(), fillStyle: "" });
    const img = await qrPng("hello", 1, { load: () => import("uqr"), doc });
    expect(img.size).toBe(21 + 4);
  });

  it("fails when canvas is unavailable", async () => {
    const { doc } = fakeCanvasDoc(null);
    const encode = vi.fn().mockReturnValue({ size: 1, data: [[true]] });
    await expect(qrPng("x", 1, { load: async () => ({ encode }), doc })).rejects.toThrow(/canvas/);
  });
});
