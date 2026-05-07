import { describe, it, expect } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useDrawerSelection } from "./useDrawerSelection";

describe("useDrawerSelection", () => {
  it("starts empty", () => {
    const { result } = renderHook(() => useDrawerSelection());
    expect(result.current.size).toBe(0);
    expect(result.current.capReached).toBe(false);
    expect(result.current.has("a")).toBe(false);
  });

  it("toggle adds the id when not present", () => {
    const { result } = renderHook(() => useDrawerSelection());

    act(() => result.current.toggle("a", false));

    expect(result.current.size).toBe(1);
    expect(result.current.has("a")).toBe(true);
  });

  it("toggle removes the id when already present", () => {
    const { result } = renderHook(() => useDrawerSelection());

    act(() => result.current.toggle("a", false));
    act(() => result.current.toggle("a", false));

    expect(result.current.size).toBe(0);
    expect(result.current.has("a")).toBe(false);
  });

  it("toggle is a no-op when isMember=true even if id is not in selection", () => {
    const { result } = renderHook(() => useDrawerSelection());

    act(() => result.current.toggle("a", true));

    expect(result.current.size).toBe(0);
    expect(result.current.has("a")).toBe(false);
  });

  it("clear empties selection", () => {
    const { result } = renderHook(() => useDrawerSelection());

    act(() => result.current.toggle("a", false));
    act(() => result.current.toggle("b", false));
    act(() => result.current.clear());

    expect(result.current.size).toBe(0);
    expect(result.current.has("a")).toBe(false);
  });

  it("respects cap — toggle does not add a new id when size === cap", () => {
    const { result } = renderHook(() => useDrawerSelection(2));

    act(() => result.current.toggle("a", false));
    act(() => result.current.toggle("b", false));
    act(() => result.current.toggle("c", false));

    expect(result.current.size).toBe(2);
    expect(result.current.has("c")).toBe(false);
    expect(result.current.capReached).toBe(true);
  });

  it("at cap — already-selected ids can still be removed (toggle off)", () => {
    const { result } = renderHook(() => useDrawerSelection(2));

    act(() => result.current.toggle("a", false));
    act(() => result.current.toggle("b", false));

    expect(result.current.capReached).toBe(true);
    act(() => result.current.toggle("a", false));

    expect(result.current.size).toBe(1);
    expect(result.current.has("a")).toBe(false);
    expect(result.current.has("b")).toBe(true);
    expect(result.current.capReached).toBe(false);
  });

  it("canSelect returns false for members regardless of cap state", () => {
    const { result } = renderHook(() => useDrawerSelection());

    expect(result.current.canSelect("a", true)).toBe(false);
  });

  it("canSelect returns true for already-selected ids even when cap reached", () => {
    const { result } = renderHook(() => useDrawerSelection(2));

    act(() => result.current.toggle("a", false));
    act(() => result.current.toggle("b", false));

    expect(result.current.canSelect("a", false)).toBe(true);
  });

  it("canSelect returns false for new ids when cap reached", () => {
    const { result } = renderHook(() => useDrawerSelection(2));

    act(() => result.current.toggle("a", false));
    act(() => result.current.toggle("b", false));

    expect(result.current.canSelect("c", false)).toBe(false);
  });

  it("default cap is 200", () => {
    const { result } = renderHook(() => useDrawerSelection());

    for (let i = 0; i < 200; i++) {
      act(() => result.current.toggle(`id-${i}`, false));
    }

    expect(result.current.size).toBe(200);
    expect(result.current.capReached).toBe(true);

    act(() => result.current.toggle("id-201", false));
    expect(result.current.size).toBe(200);
  });
});
