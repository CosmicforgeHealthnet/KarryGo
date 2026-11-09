import Image from "next/image";

// The Karryam lockup from the vendor app (apps/vendor/assets/branding).
// Navy wordmark on light surfaces, white wordmark on dark ones ("onDark"
// forces the white one, e.g. on a navy panel).
export function KarryamLogo({
  height = 26,
  onDark = false,
  className = "",
}: {
  height?: number;
  onDark?: boolean;
  className?: string;
}) {
  const width = Math.round((height * 1221) / 304);
  const img = (src: string, cls: string) => (
    <Image src={src} alt="Karryam" width={width} height={height} priority className={cls} style={{ height, width: "auto" }} />
  );
  if (onDark) return <span className={`inline-flex ${className}`}>{img("/images/karryam-logo-white.png", "")}</span>;
  return (
    <span className={`inline-flex ${className}`}>
      {img("/images/karryam-logo-navy.png", "dark:hidden")}
      {img("/images/karryam-logo-white.png", "hidden dark:block")}
    </span>
  );
}
