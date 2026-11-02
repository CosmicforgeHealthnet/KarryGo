// A soft edge between two section backgrounds.
export default function SectionJoin({ from, to }: { from: string; to: string }) {
  return (
    <div
      aria-hidden
      className="h-10 sm:h-14 lg:h-20"
      style={{ background: `linear-gradient(to bottom, ${from}, ${to})` }}
    />
  );
}
