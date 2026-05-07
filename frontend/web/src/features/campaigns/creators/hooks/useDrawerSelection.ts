import { useCallback, useState } from "react";

export const SELECTION_CAP = 200;

export interface UseDrawerSelectionResult {
  selected: Set<string>;
  size: number;
  capReached: boolean;
  has: (id: string) => boolean;
  canSelect: (id: string, isMember: boolean) => boolean;
  toggle: (id: string, isMember: boolean) => void;
  clear: () => void;
}

export function useDrawerSelection(
  cap: number = SELECTION_CAP,
): UseDrawerSelectionResult {
  const [selected, setSelected] = useState<Set<string>>(() => new Set());

  const has = useCallback((id: string) => selected.has(id), [selected]);

  const capReached = selected.size >= cap;

  const canSelect = useCallback(
    (id: string, isMember: boolean) => {
      if (isMember) return false;
      if (selected.has(id)) return true;
      return selected.size < cap;
    },
    [selected, cap],
  );

  const toggle = useCallback(
    (id: string, isMember: boolean) => {
      if (isMember) return;
      setSelected((prev) => {
        const next = new Set(prev);
        if (next.has(id)) {
          next.delete(id);
          return next;
        }
        if (next.size >= cap) return prev;
        next.add(id);
        return next;
      });
    },
    [cap],
  );

  const clear = useCallback(() => {
    setSelected((prev) => (prev.size === 0 ? prev : new Set()));
  }, []);

  return {
    selected,
    size: selected.size,
    capReached,
    has,
    canSelect,
    toggle,
    clear,
  };
}
