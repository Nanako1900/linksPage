import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ErrorBoundary } from "./ErrorBoundary";

afterEach(cleanup);

function Boom(): never {
  throw new Error("render bug");
}

describe("ErrorBoundary", () => {
  it("renders children when nothing throws", () => {
    render(
      <ErrorBoundary fallback={<p>fallback</p>}>
        <p>content</p>
      </ErrorBoundary>,
    );
    expect(screen.getByText("content")).toBeTruthy();
  });

  it("renders the fallback and reports render errors", () => {
    const report = vi.fn();
    vi.stubGlobal("reportError", report);
    vi.spyOn(console, "error").mockImplementation(() => {}); // React's own caught-error log
    render(
      <ErrorBoundary fallback={<p role="alert">fallback</p>}>
        <Boom />
      </ErrorBoundary>,
    );
    expect(screen.getByRole("alert").textContent).toBe("fallback");
    expect(report).toHaveBeenCalledWith(expect.objectContaining({ message: "render bug" }));
  });
});
