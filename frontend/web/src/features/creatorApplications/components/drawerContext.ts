import { createContext, useContext } from "react";

export interface DrawerCtx {
  onApiError: (message: string) => void;
  onCloseDrawer: () => void;
}

export const DrawerContext = createContext<DrawerCtx | null>(null);

export function useDrawerContext(): DrawerCtx {
  const ctx = useContext(DrawerContext);
  if (!ctx) {
    throw new Error("useDrawerContext must be used within ApplicationDrawer");
  }
  return ctx;
}
