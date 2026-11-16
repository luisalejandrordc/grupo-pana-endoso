import { initials } from "../lib/format";

/** The initials of an organization or a customer, on a rounded square (a person gets an Avatar instead). */
export function InitialsTile({ name }: { name: string }) {
  return (
    <span className="initials-tile" aria-hidden="true">
      {initials(name)}
    </span>
  );
}
