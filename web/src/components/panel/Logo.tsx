// NullGate brand mark — the golden gate emblem from the official 3D logo
// (web/public/nullgate-logo.webp, text band cropped since the panel already
// renders the wordmark next to the mark). Rendered as a circular emblem so the
// pure-black corners of the render blend into the dark theme and the mark sits
// concentric inside the login page's orbit rings.
export default function Logo({ className = "size-10" }: { className?: string }) {
  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src="/nullgate-logo.webp"
      alt="NullGate"
      className={`${className} rounded-full object-cover select-none ring-1 ring-gold/25`}
    />
  );
}
