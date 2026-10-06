import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { ChannelView, UserView } from "../../shared/types/public";
import { renderWithEnv } from "../../test/render";
import { Channels, Members, STACK_MAX } from "./Members";

afterEach(cleanup);

const user = (i: number, patch: Partial<UserView> = {}): UserView => ({
  name: `user${i}`,
  avatarUrl: `/media/p/QXZhdGFyMDAwMDAwMDAw${i}.png`,
  status: "online",
  ...patch,
});

describe("Members", () => {
  it("renders nothing when hidden or empty", () => {
    const { container } = renderWithEnv(<Members users={[user(1)]} display="hidden" />);
    expect(container.innerHTML).toBe("");
    cleanup();
    const again = renderWithEnv(<Members users={[]} display="avatars_names" />);
    expect(again.container.innerHTML).toBe("");
  });

  it("collapses to at most five avatars plus +N", () => {
    const users = Array.from({ length: 8 }, (_, i) => user(i));
    const { container } = renderWithEnv(<Members users={users} display="avatars_names" />);
    const summary = container.querySelector("summary") as HTMLElement;
    expect(summary.querySelectorAll("img")).toHaveLength(STACK_MAX);
    expect(summary.textContent).toContain("+3");
    expect(container.querySelector("details")?.open).toBe(false);
  });

  it("avatars_names lists avatar and nickname with status", () => {
    renderWithEnv(
      <Members
        users={[user(1), user(2, { status: "idle", name: "猎人二号" }), user(3, { status: "dnd", avatarUrl: null })]}
        display="avatars_names"
      />,
    );
    const list = screen.getByRole("list", { name: "在线成员" });
    const items = within(list).getAllByRole("listitem");
    expect(items.map((li) => li.textContent)).toEqual(["user1在线", "猎人二号离开", "user3请勿打扰"]);
    expect(items[2]?.querySelector("img")).toBeNull();
    expect(items[1]?.querySelector(".lp-status-idle")).toBeTruthy();
    expect(list.querySelector("img")?.getAttribute("width")).toBe("32");
  });

  it("avatars mode shows no names", () => {
    renderWithEnv(<Members users={[user(1, { name: null }), user(2, { name: null })]} display="avatars" />);
    const list = screen.getByRole("list", { name: "在线成员" });
    expect(list.querySelectorAll(".truncate")).toHaveLength(0);
    expect(list.querySelectorAll("img")).toHaveLength(2);
    expect(screen.queryByText("+0")).toBeNull();
  });
});

describe("Channels", () => {
  const ch = (i: number): ChannelView => ({ id: String(i), name: `room-${i}` });

  it("renders nothing without channels", () => {
    const { container } = renderWithEnv(<Channels channels={[]} />);
    expect(container.innerHTML).toBe("");
  });

  it("shows up to six chips and +N", () => {
    renderWithEnv(<Channels channels={Array.from({ length: 9 }, (_, i) => ch(i))} />, { locale: "en" });
    const items = within(screen.getByRole("list", { name: "Channels" })).getAllByRole("listitem");
    expect(items).toHaveLength(7);
    expect(items[0]?.textContent).toBe("#room-0");
    expect(items[6]?.textContent).toBe("+3");
  });
});
