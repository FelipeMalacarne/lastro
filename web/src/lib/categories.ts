import { useMemo } from "react";

import { useResource, type Category } from "@/lib/api";

/** Every category by id (archived ones too, so old transactions keep theirs), and the live ones in order. */
export function useCategories(): { byId: Map<string, Category>; live: Category[]; loading: boolean } {
  const res = useResource<{ categories: Category[] }>("/api/v1/categories");
  return useMemo(() => {
    const all = res.data?.categories ?? [];
    return { byId: new Map(all.map((c) => [c.id, c])), live: all.filter((c) => !c.archived), loading: res.loading };
  }, [res.data, res.loading]);
}

/** How many transactions wait for a category, for the badges. */
export function useWaiting(): number | undefined {
  return useResource<{ waiting: number }>("/api/v1/review").data?.waiting;
}
