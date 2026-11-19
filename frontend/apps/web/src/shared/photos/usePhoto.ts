import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";

/** The address of a signed address lives about ten minutes; it is reused for eight so a screen never shows a dead one. */
const REUSE_MS = 8 * 60 * 1000;

export const photoKey = (employeeId: string) =>
  ["employee-photo", employeeId] as const;

/**
 * The picture address of an employee. Nothing is requested unless the employee is known to have a picture
 * (`hasPhoto` comes with /me, the employee records, the advisor picker and the sales), so people without one cost no request.
 */
export function usePhoto(employeeId: string | undefined, hasPhoto: boolean) {
  return useQuery({
    queryKey: photoKey(employeeId ?? ""),
    queryFn: () => api.employeePhoto(employeeId as string),
    enabled: Boolean(employeeId) && hasPhoto,
    staleTime: REUSE_MS,
    gcTime: 2 * REUSE_MS,
    retry: false,
    select: (link) => link.url,
  });
}

/** Forgets the stored address of an employee, for after their picture changed. */
export function useForgetPhoto() {
  const client = useQueryClient();
  return (employeeId: string) =>
    client.invalidateQueries({ queryKey: photoKey(employeeId) });
}
