// NullGate brand mark — vector version of /nullgate-logo.svg (open gate + null
// slash). Inline SVG keeps it crisp at every size and lets the glow follow the
// theme via currentColor-free gradients baked into the markup.
export default function Logo({ className = "size-10" }: { className?: string }) {
  return (
    <svg viewBox="0 0 64 64" fill="none" className={className} aria-hidden="true">
      <defs>
        <linearGradient id="ngGoldG" x1="14" y1="8" x2="50" y2="56" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor="#f9e29a" />
          <stop offset=".45" stopColor="#e8bc4f" />
          <stop offset="1" stopColor="#a9761f" />
        </linearGradient>
        <radialGradient id="ngGlowG" cx=".5" cy=".42" r=".62">
          <stop offset="0" stopColor="#ffd970" stopOpacity=".5" />
          <stop offset="1" stopColor="#ffd970" stopOpacity="0" />
        </radialGradient>
      </defs>
      <circle cx="32" cy="30" r="27" fill="url(#ngGlowG)" />
      <path d="M13.5 52.5 V27 a18.5 18.5 0 0 1 37 0 V52.5" stroke="url(#ngGoldG)" strokeWidth="4.6" strokeLinecap="round" />
      <path d="M8.5 52.5 h47" stroke="url(#ngGoldG)" strokeWidth="4.6" strokeLinecap="round" />
      <path d="M22.5 52.5 V31 a9.5 9.5 0 0 1 19 0 V52.5" stroke="url(#ngGoldG)" strokeWidth="2.4" strokeLinecap="round" opacity=".5" />
      <path d="M27.5 43.5 L36.5 25.5" stroke="url(#ngGoldG)" strokeWidth="3.6" strokeLinecap="round" />
      <circle cx="50.5" cy="15.5" r="1.7" fill="#f4d276" opacity=".95" />
      <circle cx="12.5" cy="19.5" r="1.2" fill="#f4d276" opacity=".7" />
      <circle cx="44" cy="8.5" r="1" fill="#f4d276" opacity=".55" />
      <circle cx="19" cy="9.5" r=".9" fill="#f4d276" opacity=".45" />
    </svg>
  );
}
