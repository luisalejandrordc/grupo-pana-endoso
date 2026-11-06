-- +goose Up
-- A short, human-friendly number for each sale ("EF-2026-00419"), so people can refer to a sale without a UUID
-- and the typed confirmation of a permanent deletion has something to type. The year in the code comes from the
-- creation date; the number itself never resets and is never reused.
ALTER TABLE sale_registers
  ADD COLUMN sale_number bigint GENERATED ALWAYS AS IDENTITY;

CREATE UNIQUE INDEX ux_sale_registers_number ON sale_registers (sale_number);
