type MascotProps = {
  className?: string;
};

/**
 * Maskot robot ALGENTRA — port verbatim dari macro `algentra_mascot`
 * di apps/core-api/app/templates/_cs_mascot.html (backup stabil added2df).
 */
export default function Mascot({ className }: MascotProps) {
  return (
    <svg className={className} viewBox="0 0 48 48" focusable="false" aria-hidden="true">
      <path d="M24 11V8" stroke="#16466e" strokeWidth="2.4" strokeLinecap="round" />
      <path
        d="M24 9c-1-4-5-5-7-3-1.5 2.3 1 4.1 7 4m0-1c1-4 5-5 7-3 1.5 2.3-1 4.1-7 4"
        fill="#4da3e8"
        stroke="#16466e"
        strokeWidth="1.1"
        strokeLinejoin="round"
      />
      <path d="M8 24H5m38 0h-3" stroke="#16466e" strokeWidth="2.2" strokeLinecap="round" />
      <circle cx="4.5" cy="24" r="2.2" fill="#f2c438" stroke="#16466e" strokeWidth="1" />
      <circle cx="43.5" cy="24" r="2.2" fill="#f2c438" stroke="#16466e" strokeWidth="1" />
      <rect x="8" y="12" width="32" height="29" rx="12" fill="#e4f0fb" stroke="#16466e" strokeWidth="1.6" />
      <rect x="13" y="17" width="22" height="16" rx="7" fill="#0f2f4d" />
      <circle cx="20" cy="23" r="2" fill="#7cc4f5" />
      <circle cx="28" cy="23" r="2" fill="#7cc4f5" />
      <path
        d="M20 28c1.1 1.2 2.4 1.8 4 1.8s2.9-.6 4-1.8"
        fill="none"
        stroke="#7cc4f5"
        strokeWidth="1.5"
        strokeLinecap="round"
      />
      <circle cx="14.5" cy="29" r="1.7" fill="#9cc9ee" opacity=".7" />
      <circle cx="33.5" cy="29" r="1.7" fill="#9cc9ee" opacity=".7" />
      <path d="M18 41v2m12-2v2m-14 1h16" stroke="#16466e" strokeWidth="1.8" strokeLinecap="round" />
      <path d="m36 9 1-2m2 4 2-1" stroke="#f2c438" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  );
}
