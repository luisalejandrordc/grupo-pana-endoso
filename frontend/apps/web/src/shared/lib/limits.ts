/**
 * How long free text may be, in characters. The first four mirror what the server enforces (the contract's
 * `maxLength`, and the validators behind it); the box shows them as a counter so nobody learns a limit by being
 * refused. A text the box accepts is never longer than the server's count, which counts characters, not units.
 */
export const LIMITS = {
  /** The notes of a sale and of a task. */
  notes: 2000,
  /** The reason for a modification (restart, reopen, correction, reassignment, deletion). */
  reason: 500,
  /** The detail of why a sale was cancelled. */
  cancelReason: 1000,
  /** The description of a vehicle model. */
  description: 1000,
} as const;
