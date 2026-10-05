import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { createT } from "../shared/i18n/t";
import { sampleBootstrap } from "../test/fixtures";
import { App } from "./App";
import { ErrorNotice } from "./ErrorNotice";

afterEach(cleanup);

describe("App", () => {
  it("renders the localized site title and description", () => {
    render(<App data={sampleBootstrap()} route={{ kind: "home" }} locale="en" t={createT("en", "zh-CN")} />);
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("My Communities");
    expect(screen.getByText("Join our communities.")).toBeTruthy();
    expect(screen.getByRole("region", { name: /Communities/ })).toBeTruthy();
    expect(screen.queryByText(/Back to home/)).toBeNull();
  });

  it("renders language links with the current locale marked", () => {
    render(<App data={sampleBootstrap()} route={{ kind: "home" }} locale="zh-CN" t={createT("zh-CN", "zh-CN")} />);
    const current = screen.getByRole("link", { current: true });
    expect(current.getAttribute("href")).toBe("?lang=zh-CN");
    expect(current.getAttribute("hreflang")).toBe("zh-CN");
    expect(current.textContent).not.toBe("zh-CN");
    expect(screen.getByRole("link", { name: "English" }).getAttribute("lang")).toBe("en");
    expect(screen.getByRole("navigation", { name: "语言" })).toBeTruthy();
  });

  it("hides decorative glyphs from assistive technology", () => {
    render(<App data={sampleBootstrap()} route={{ kind: "privacy" }} locale="en" t={createT("en", "en")} />);
    expect(screen.getByRole("region").getAttribute("aria-labelledby")).toBeTruthy();
    expect(screen.getByRole("heading", { level: 2 }).querySelector(".tabular-nums")?.getAttribute("aria-hidden")).toBe(
      "true",
    );
    expect(screen.getByRole("link", { name: "Back to home" })).toBeTruthy();
  });

  it("keeps a non-default language on the home link", () => {
    render(<App data={sampleBootstrap()} route={{ kind: "privacy" }} locale="en" t={createT("en", "zh-CN")} />);
    expect(screen.getByRole("link", { name: /Back to home/ }).getAttribute("href")).toBe("/?lang=en");
  });

  it("renders the not-found copy for unknown dev-server paths", () => {
    render(<App data={sampleBootstrap()} route={{ kind: "notFound" }} locale="en" t={createT("en", "en")} />);
    expect(screen.getByText("This page does not exist.")).toBeTruthy();
    expect(screen.queryByRole("region")).toBeNull();
  });

  it("renders the privacy page with a home link", () => {
    const data = { ...sampleBootstrap(), site: { ...sampleBootstrap().site, defaultLocale: "en" } };
    render(<App data={data} route={{ kind: "privacy" }} locale="en" t={createT("en", "en")} />);
    expect(screen.getByRole("region", { name: "Privacy" })).toBeTruthy();
    expect(screen.getByRole("link", { name: /Back to home/ }).getAttribute("href")).toBe("/");
  });

  it("renders a community page placeholder", () => {
    const data = { ...sampleBootstrap(), site: { ...sampleBootstrap().site, locales: null } };
    render(<App data={data} route={{ kind: "community", slug: "abc" }} locale="en" t={createT("en", "en")} />);
    expect(screen.getByText("/c/abc")).toBeTruthy();
  });

  it("renders the error notice as an alert", () => {
    render(<ErrorNotice message="boom" />);
    expect(screen.getByRole("alert").textContent).toBe("boom");
  });
});
