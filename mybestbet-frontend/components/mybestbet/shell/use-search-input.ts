"use client"

import { useShell } from "./shell-context"

/** Holds the text typed into a search bar; results show in the bar's own dropdown and never filter the page. */
export function useSearchInput() {
  const shell = useShell()
  return (value: string) => shell.setQ(value)
}
