import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

type Option<T extends string> = { value: T; label: string };

type Props<T extends string> = {
  label: string;
  options: Option<T>[];
  value: T;
  onChange: (v: T) => void;
  className?: string;
};

/** One choice among a few, always one selected. Arrow keys move between segments. */
export function SegmentedControl<T extends string>({ label, options, value, onChange, className }: Props<T>) {
  return (
    <ToggleGroup
      type="single"
      aria-label={label}
      value={value}
      // Radix lets a single group be emptied; a segmented control never is.
      onValueChange={(v) => v && onChange(v as T)}
      className={className}
    >
      {options.map((o) => (
        <ToggleGroupItem key={o.value} value={o.value}>
          {o.label}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
