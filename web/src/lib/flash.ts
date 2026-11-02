import { useCallback, useEffect, useRef, useState } from "react";

/**
 * A confirmation that stays on screen for a few seconds ("Saved."), even
 * when what it confirms reloads underneath it. Keep it above whatever
 * remounts after a save.
 */
export function useFlash(ms = 5000): [boolean, () => void] {
  const [on, setOn] = useState(false);
  const timer = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearTimeout(timer.current), []);
  const flash = useCallback(() => {
    window.clearTimeout(timer.current);
    setOn(true);
    timer.current = window.setTimeout(() => setOn(false), ms);
  }, [ms]);
  return [on, flash];
}
