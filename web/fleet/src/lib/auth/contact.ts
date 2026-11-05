// lib/auth/contact.ts
//
// Email and phone validation for the sign-up and sign-in forms.
//
// The rules mirror services/fleet-partner-service/internal/platform/contact:
// the server accepts "+" followed by 8-15 digits. The form always sends that
// E.164 form, built from an explicit country choice, so the server never has
// to guess which country a national number belongs to.
//
// Kept free of imports so it can be unit-tested with Node's built-in runner.

export interface PhoneCountry {
  iso: string;
  name: string;
  flag: string;
  dialCode: string;
  // Allowed lengths of the national significant number (after the country
  // code, without any trunk prefix).
  minLength: number;
  maxLength: number;
  // Digit dialled before a national number inside the country ("0" in
  // Nigeria). Stripped when building the international number.
  trunkPrefix?: string;
}

export const PHONE_COUNTRIES: PhoneCountry[] = [
  { iso: "NG", name: "Nigeria", flag: "🇳🇬", dialCode: "234", minLength: 10, maxLength: 10, trunkPrefix: "0" },
  { iso: "GH", name: "Ghana", flag: "🇬🇭", dialCode: "233", minLength: 9, maxLength: 9, trunkPrefix: "0" },
  { iso: "KE", name: "Kenya", flag: "🇰🇪", dialCode: "254", minLength: 9, maxLength: 9, trunkPrefix: "0" },
  { iso: "ZA", name: "South Africa", flag: "🇿🇦", dialCode: "27", minLength: 9, maxLength: 9, trunkPrefix: "0" },
  { iso: "EG", name: "Egypt", flag: "🇪🇬", dialCode: "20", minLength: 9, maxLength: 10, trunkPrefix: "0" },
  { iso: "CM", name: "Cameroon", flag: "🇨🇲", dialCode: "237", minLength: 9, maxLength: 9 },
  { iso: "CI", name: "Côte d'Ivoire", flag: "🇨🇮", dialCode: "225", minLength: 10, maxLength: 10 },
  { iso: "SN", name: "Senegal", flag: "🇸🇳", dialCode: "221", minLength: 9, maxLength: 9 },
  { iso: "RW", name: "Rwanda", flag: "🇷🇼", dialCode: "250", minLength: 9, maxLength: 9, trunkPrefix: "0" },
  { iso: "UG", name: "Uganda", flag: "🇺🇬", dialCode: "256", minLength: 9, maxLength: 9, trunkPrefix: "0" },
  { iso: "TZ", name: "Tanzania", flag: "🇹🇿", dialCode: "255", minLength: 9, maxLength: 9, trunkPrefix: "0" },
  { iso: "ET", name: "Ethiopia", flag: "🇪🇹", dialCode: "251", minLength: 9, maxLength: 9, trunkPrefix: "0" },
  { iso: "GB", name: "United Kingdom", flag: "🇬🇧", dialCode: "44", minLength: 9, maxLength: 10, trunkPrefix: "0" },
  { iso: "US", name: "United States / Canada", flag: "🇺🇸", dialCode: "1", minLength: 10, maxLength: 10 },
  { iso: "AE", name: "United Arab Emirates", flag: "🇦🇪", dialCode: "971", minLength: 8, maxLength: 9, trunkPrefix: "0" },
  { iso: "IN", name: "India", flag: "🇮🇳", dialCode: "91", minLength: 10, maxLength: 10, trunkPrefix: "0" },
  { iso: "DE", name: "Germany", flag: "🇩🇪", dialCode: "49", minLength: 6, maxLength: 11, trunkPrefix: "0" },
  { iso: "FR", name: "France", flag: "🇫🇷", dialCode: "33", minLength: 9, maxLength: 9, trunkPrefix: "0" },
  { iso: "CN", name: "China", flag: "🇨🇳", dialCode: "86", minLength: 10, maxLength: 11, trunkPrefix: "0" },
];

export const DEFAULT_PHONE_COUNTRY = "NG";

export function findPhoneCountry(iso: string): PhoneCountry {
  return PHONE_COUNTRIES.find((c) => c.iso === iso) ?? PHONE_COUNTRIES[0];
}

// Mirrors the server's email pattern: one "@", no spaces, a dotted domain.
const EMAIL_PATTERN = /^[^@\s]+@[^@\s.]+(\.[^@\s.]+)+$/;

export type FieldIssue = "missing" | "invalid";

export function emailIssue(value: string): FieldIssue | null {
  const trimmed = value.trim();
  if (trimmed === "") return "missing";
  if (trimmed.length > 254 || !EMAIL_PATTERN.test(trimmed.toLowerCase())) return "invalid";
  return null;
}

export type PhoneResult =
  | { ok: true; e164: string }
  | { ok: false; issue: FieldIssue };

const SEPARATORS = /[\s().-]/g;

// normalizePhone turns what the user typed into E.164 for the chosen country.
//
// - "+<digits>" or "00<digits>" is taken as a full international number, as
//   typed. The selected country is not consulted and never changed.
// - Anything else is a national number for the selected country: a leading
//   trunk prefix ("0") is dropped and the country code is prepended. If the
//   user already typed the country code without "+" ("2348012345678") it is
//   not added a second time.
export function normalizePhone(raw: string, countryIso: string): PhoneResult {
  const typed = raw.trim();
  if (typed === "") return { ok: false, issue: "missing" };

  const compact = typed.replace(SEPARATORS, "");
  let international: string | null = null;
  if (compact.startsWith("+")) international = compact.slice(1);
  else if (compact.startsWith("00")) international = compact.slice(2);

  if (international !== null) {
    if (!/^[0-9]+$/.test(international)) return { ok: false, issue: "invalid" };
    return checkInternational(international);
  }

  if (!/^[0-9]+$/.test(compact)) return { ok: false, issue: "invalid" };

  const country = findPhoneCountry(countryIso);
  let national = compact;
  if (country.trunkPrefix && national.startsWith(country.trunkPrefix)) {
    national = national.slice(country.trunkPrefix.length);
  } else if (
    national.startsWith(country.dialCode) &&
    fitsCountry(national.slice(country.dialCode.length), country)
  ) {
    // Country code typed without "+".
    national = national.slice(country.dialCode.length);
  }

  if (!fitsCountry(national, country)) return { ok: false, issue: "invalid" };
  return { ok: true, e164: `+${country.dialCode}${national}` };
}

function fitsCountry(national: string, country: PhoneCountry): boolean {
  return national.length >= country.minLength && national.length <= country.maxLength && !national.startsWith("0");
}

// checkInternational validates "+<digits>". For a known country it also
// checks the national length and drops a trunk "0" typed after the country
// code ("+2340801..." is a common slip for "+234801...").
function checkInternational(digits: string): PhoneResult {
  // Longest dial code first so "+1..." never shadows a longer match.
  const match = [...PHONE_COUNTRIES]
    .sort((a, b) => b.dialCode.length - a.dialCode.length)
    .find((c) => digits.startsWith(c.dialCode));
  if (match) {
    let national = digits.slice(match.dialCode.length);
    if (match.trunkPrefix && national.startsWith(match.trunkPrefix) && fitsCountry(national.slice(match.trunkPrefix.length), match)) {
      national = national.slice(match.trunkPrefix.length);
    }
    if (!fitsCountry(national, match)) return { ok: false, issue: "invalid" };
    return { ok: true, e164: `+${match.dialCode}${national}` };
  }
  if (digits.length < 8 || digits.length > 15 || digits.startsWith("0")) return { ok: false, issue: "invalid" };
  return { ok: true, e164: `+${digits}` };
}

export function maskEmail(value: string): string {
  const normalized = value.trim().toLowerCase();
  const at = normalized.lastIndexOf("@");
  if (at <= 0) return "***";
  const local = normalized.slice(0, at);
  return `${local.slice(0, 1)}***@${normalized.slice(at + 1)}`;
}
