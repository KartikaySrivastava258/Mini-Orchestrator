import { age, exactTime, relativeTime } from "@/lib/format";

/** "5m ago", with the exact timestamp on hover for the detail people. */
export function RelativeTime({ value, short = false }: { value?: string; short?: boolean }) {
  const label = short ? age(value) : relativeTime(value);
  return (
    <time dateTime={value} title={exactTime(value)} className="whitespace-nowrap tabular-nums">
      {label}
    </time>
  );
}
