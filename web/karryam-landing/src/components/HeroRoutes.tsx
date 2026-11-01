// Moving lights traced over hero-movement-map-v2.png, in the image's own
// coordinates, so they stay aligned at every size. Decorative only.
const ROUTES = [
  { d: "M797 672 L880 612 L935 640 L985 548", dur: "3.6s", delay: "1s" },
  { d: "M660 395 C560 360 440 345 335 352", dur: "5s", delay: "1.6s" },
  { d: "M1040 450 C1130 405 1240 405 1340 425", dur: "5.4s", delay: "2.4s" },
  { d: "M550 860 L565 860 M848 860 L865 860 M1150 860 L1165 860", dur: "4.4s", delay: "3s" },
];

const NODES = [
  { x: 985, y: 548, delay: "1s" },
  { x: 797, y: 672, delay: "2.2s" },
  { x: 622, y: 250, delay: "1.5s" },
  { x: 1630, y: 335, delay: "2.7s" },
];

export default function HeroRoutes() {
  return (
    <svg
      aria-hidden
      viewBox="0 0 1672 941"
      fill="none"
      className="hero-routes pointer-events-none absolute inset-0 h-full w-full"
    >
      {ROUTES.map(({ d, dur, delay }) => (
        <path
          key={d}
          d={d}
          pathLength={1}
          stroke="#7CF5E4"
          strokeWidth={5}
          strokeLinecap="round"
          className="hero-route"
          style={{ "--dur": dur, "--delay": delay } as React.CSSProperties}
        />
      ))}
      {NODES.map(({ x, y, delay }) => (
        <circle
          key={`${x}-${y}`}
          cx={x}
          cy={y}
          r={18}
          stroke="#00C2A8"
          strokeWidth={3}
          className="hero-node"
          style={{ "--delay": delay } as React.CSSProperties}
        />
      ))}
    </svg>
  );
}
