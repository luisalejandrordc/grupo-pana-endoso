const day = new Intl.DateTimeFormat("es-PE", {
  day: "numeric",
  month: "long",
  year: "numeric",
  // A calendar date (YYYY-MM-DD) has no time zone: reading it as UTC keeps it on the day it names.
  timeZone: "UTC",
});

/** "2026-03-14" → "14 de marzo de 2026". Never shifts the day, whatever the viewer's time zone. */
export function formatDay(value: string): string {
  const match = /^(\d{4})-(\d{2})-(\d{2})/.exec(value);
  if (!match) return value;
  return day.format(
    new Date(
      Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3])),
    ),
  );
}

/** A moment (ISO 8601 in UTC) as a date in Lima, long form. */
export function formatMoment(value: string): string {
  return new Intl.DateTimeFormat("es-PE", {
    day: "numeric",
    month: "long",
    year: "numeric",
    timeZone: "America/Lima",
  }).format(new Date(value));
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1).replace(".0", "")} MB`;
}

/**
 * A WhatsApp chat link for a phone number, when it can be one: a Peruvian mobile (nine digits starting with 9) or
 * a number that already carries its country code. Anything else gets no link rather than a wrong one.
 */
export function whatsappLink(phone: string): string | null {
  const digits = phone.replace(/\D/g, "");
  if (/^9\d{8}$/.test(digits)) return `https://wa.me/51${digits}`;
  if (phone.trim().startsWith("+") && digits.length >= 10)
    return `https://wa.me/${digits}`;
  if (/^519\d{8}$/.test(digits)) return `https://wa.me/${digits}`;
  return null;
}
