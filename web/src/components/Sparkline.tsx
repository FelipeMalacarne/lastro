type Props = {
  values: number[];
  width?: number;
  height?: number;
  label: string;
  /** Start the scale at zero (the default); off, the line spans its own range. */
  zero?: boolean;
};

/** A small line of a few values, with the last one marked. */
export function Sparkline({ values, width = 120, height = 32, label, zero = true }: Props) {
  if (values.length < 2) return null;
  const pad = 3;
  const max = Math.max(...values);
  const min = zero ? Math.min(...values, 0) : Math.min(...values);
  const span = max - min || 1;
  const x = (i: number) => pad + (i * (width - pad * 2)) / (values.length - 1);
  const y = (v: number) => height - pad - ((v - min) / span) * (height - pad * 2);
  const d = values.map((v, i) => `${i ? "L" : "M"}${x(i).toFixed(1)} ${y(v).toFixed(1)}`).join(" ");
  const last = values.length - 1;
  return (
    <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} role="img" aria-label={label} className="flex-none overflow-visible">
      <path d={d} fill="none" stroke="var(--tinta)" strokeWidth={1.5} strokeLinejoin="round" />
      <circle cx={x(last)} cy={y(values[last])} r={2.5} fill="var(--tinta)" />
    </svg>
  );
}
