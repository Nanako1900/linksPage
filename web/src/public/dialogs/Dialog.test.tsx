import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IDS, sampleCommunity } from "../../test/fixtures";
import { renderWithEnv } from "../../test/render";
import { Dialog } from "./Dialog";
import { DialogHost } from "./DialogHost";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

/** The dialog box used for backdrop hit-testing (jsdom has no layout). */
const BOX = DOMRect.fromRect({ x: 100, y: 100, width: 300, height: 200 });
const INSIDE = { clientX: 110, clientY: 110 };
const OUTSIDE = { clientX: 20, clientY: 20 };

function pressAndClick(el: HTMLElement, at: { clientX: number; clientY: number }) {
  fireEvent.mouseDown(el, at);
  fireEvent.click(el, at);
}

describe("Dialog", () => {
  it("opens modally, is labelled by its title and closes via the button", async () => {
    const showModal = vi.spyOn(HTMLDialogElement.prototype, "showModal");
    const onClose = vi.fn();
    render(
      <Dialog title="Hello" closeLabel="Close" onClose={onClose}>
        <p>body</p>
      </Dialog>,
    );
    const dialog = screen.getByRole("dialog", { name: "Hello" });
    expect(showModal).toHaveBeenCalledOnce();
    expect(dialog.hasAttribute("open")).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    await waitFor(() => expect(onClose).toHaveBeenCalledOnce());
    expect(dialog.hasAttribute("open")).toBe(false);
  });

  it("closes on Escape and on backdrop clicks", async () => {
    const onClose = vi.fn();
    render(
      <Dialog title="T" closeLabel="Close" onClose={onClose}>
        <p>body</p>
      </Dialog>,
    );
    const dialog = screen.getByRole("dialog");
    fireEvent.keyDown(dialog, { key: "Escape" });
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    dialog.setAttribute("open", "");
    fireEvent.click(screen.getByText("body"));
    expect(onClose).toHaveBeenCalledTimes(1);
    vi.spyOn(dialog, "getBoundingClientRect").mockReturnValue(BOX);
    pressAndClick(dialog, OUTSIDE);
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(2));
  });

  it.each([
    ["the dialog padding", (d: HTMLElement) => pressAndClick(d, INSIDE)],
    ["the content", () => pressAndClick(screen.getByText("body"), INSIDE)],
    [
      "a drag that ends on the backdrop",
      (d: HTMLElement) => {
        fireEvent.mouseDown(screen.getByText("body"), INSIDE);
        fireEvent.click(d, OUTSIDE);
      },
    ],
    [
      "a drag that starts on the backdrop and ends inside",
      (d: HTMLElement) => {
        fireEvent.mouseDown(d, OUTSIDE);
        fireEvent.click(d, INSIDE);
      },
    ],
  ])("does not close on clicks on %s", (_name, act) => {
    const onClose = vi.fn();
    render(
      <Dialog title="T" closeLabel="Close" onClose={onClose}>
        <p>body</p>
      </Dialog>,
    );
    const dialog = screen.getByRole("dialog");
    vi.spyOn(dialog, "getBoundingClientRect").mockReturnValue(BOX);
    act(dialog);
    expect(onClose).not.toHaveBeenCalled();
    expect(dialog.hasAttribute("open")).toBe(true);
  });

  it("overlay variant ignores backdrop clicks and can hide its title", () => {
    const onClose = vi.fn();
    render(
      <Dialog title="Guide" closeLabel="Close" onClose={onClose} variant="overlay">
        <p>x</p>
      </Dialog>,
    );
    const dialog = screen.getByRole("dialog", { name: "Guide" });
    expect(dialog.className).toBe("lp-overlay");
    expect(screen.getByText("Guide").className).toBe("sr-only");
    vi.spyOn(dialog, "getBoundingClientRect").mockReturnValue(BOX);
    pressAndClick(dialog, OUTSIDE);
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("hides the title on request", () => {
    render(
      <Dialog title="Quiet" hideTitle closeLabel="Close" onClose={vi.fn()}>
        <p>x</p>
      </Dialog>,
    );
    expect(screen.getByText("Quiet").className).toBe("sr-only");
  });

  it("falls back to the open attribute without showModal", () => {
    const proto = HTMLDialogElement.prototype as unknown as Record<string, unknown>;
    const { showModal, close } = proto;
    proto.showModal = undefined;
    proto.close = undefined;
    try {
      const onClose = vi.fn();
      render(
        <Dialog title="Old" closeLabel="Close" onClose={onClose}>
          <p>x</p>
        </Dialog>,
      );
      expect(screen.getByRole("dialog").hasAttribute("open")).toBe(true);
      fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
      expect(onClose).toHaveBeenCalledOnce();
    } finally {
      proto.showModal = showModal;
      proto.close = close;
    }
  });
});

describe("DialogHost", () => {
  it("renders nothing without a spec", () => {
    const { container } = renderWithEnv(<DialogHost spec={null} onClose={vi.fn()} />);
    expect(container.innerHTML).toBe("");
  });

  it("open-in-browser overlay: guide text (admin copy), arrow and copy link", () => {
    renderWithEnv(
      <DialogHost spec={{ kind: "browser", url: "https://links.example.com/go/discord" }} onClose={vi.fn()} />,
    );
    const dialog = screen.getByRole("dialog", { name: "请在浏览器中打开" });
    expect(dialog.textContent).toContain("点右上角 ··· 选择「在浏览器打开」");
    expect(dialog.querySelector(".lp-overlay-arrow")).toBeTruthy();
    expect(dialog.textContent).toContain("https://links.example.com/go/discord");
    expect(screen.getByRole("button", { name: /复制链接/ })).toBeTruthy();
  });

  it("open-in-browser uses the built-in English guide without an en override", () => {
    renderWithEnv(<DialogHost spec={{ kind: "browser", url: "u" }} onClose={vi.fn()} />, { locale: "en" });
    // site.copy only overrides zh-CN, which is the default locale → it wins (server order).
    expect(screen.getByRole("dialog").textContent).toContain("点右上角");
  });

  it("open-on-desktop shows the address in large text", () => {
    renderWithEnv(
      <DialogHost spec={{ kind: "desktop", url: "https://links.example.com/c/discord" }} onClose={vi.fn()} />,
    );
    const dialog = screen.getByRole("dialog", { name: "在电脑上打开" });
    expect(dialog.querySelector(".text-2xl.select-all")?.textContent).toBe("https://links.example.com/c/discord");
    expect(dialog.textContent).toContain("在电脑浏览器中打开下面的地址");
  });

  it("uploaded QR shows a PNG image with note", () => {
    const qr = sampleCommunity(IDS.wechat).qr;
    if (!qr) throw new Error("fixture");
    renderWithEnv(<DialogHost spec={{ kind: "qr", title: "微信群", qr }} onClose={vi.fn()} />);
    const img = screen.getByRole("img", { name: "二维码" });
    expect(img.getAttribute("src")).toBe(qr.url);
    expect(screen.getByRole("dialog").textContent).toContain("长按识别二维码 · 更新于 10-05");
  });

  it("uploaded QR without note shows only the hint", () => {
    renderWithEnv(
      <DialogHost
        spec={{ kind: "qr", title: "Q", qr: { url: "/media/q/x", width: 1, height: 1, note: {} } }}
        onClose={vi.fn()}
      />,
    );
    expect(screen.getByRole("dialog").textContent).toContain("长按识别二维码");
  });

  it("generated QR shows a skeleton, then the PNG", async () => {
    let resolve: (v: { src: string; size: number }) => void = () => {};
    const makeQr = vi.fn().mockReturnValue(new Promise((r) => (resolve = r)));
    renderWithEnv(
      <DialogHost spec={{ kind: "qrGenerate", title: "D", url: "https://x/go/d" }} onClose={vi.fn()} makeQr={makeQr} />,
    );
    expect(screen.getByRole("dialog").querySelector(".lp-skeleton")).toBeTruthy();
    await act(async () => resolve({ src: "data:image/png;base64,AA", size: 264 }));
    const img = screen.getByRole("img", { name: "二维码" });
    expect(img.getAttribute("src")).toBe("data:image/png;base64,AA");
    expect(img.getAttribute("width")).toBe("264");
    expect(makeQr).toHaveBeenCalledWith("https://x/go/d");
  });

  it("generated QR failure shows a message", async () => {
    const makeQr = vi.fn().mockRejectedValue(new Error("no canvas"));
    renderWithEnv(<DialogHost spec={{ kind: "qrGenerate", title: "D", url: "u" }} onClose={vi.fn()} makeQr={makeQr} />);
    expect(await screen.findByText("二维码生成失败。")).toBeTruthy();
  });
});
