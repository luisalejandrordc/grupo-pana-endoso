-- +goose Up
CREATE TABLE employee_refresh_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_id uuid NOT NULL REFERENCES employees (id),
  token_hash text NOT NULL UNIQUE,
  family_id uuid NOT NULL,
  parent_token_id uuid REFERENCES employee_refresh_tokens (id),
  remember_me boolean NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  absolute_expires_at timestamptz NOT NULL,
  rotated_at timestamptz,
  revoked_at timestamptz,
  revoked_reason revoked_reason,
  CONSTRAINT employee_refresh_tokens_revocation CHECK ((revoked_at IS NULL) = (revoked_reason IS NULL)),
  CONSTRAINT employee_refresh_tokens_expiry CHECK (expires_at <= absolute_expires_at)
);

CREATE INDEX ix_employee_refresh_tokens_family ON employee_refresh_tokens (family_id);

CREATE INDEX ix_employee_refresh_tokens_employee ON employee_refresh_tokens (employee_id);

CREATE INDEX ix_employee_refresh_tokens_expires ON employee_refresh_tokens (expires_at);

CREATE TABLE customer_refresh_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_id uuid NOT NULL REFERENCES customers (id),
  token_hash text NOT NULL UNIQUE,
  family_id uuid NOT NULL,
  parent_token_id uuid REFERENCES customer_refresh_tokens (id),
  remember_me boolean NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  absolute_expires_at timestamptz NOT NULL,
  rotated_at timestamptz,
  revoked_at timestamptz,
  revoked_reason revoked_reason,
  CONSTRAINT customer_refresh_tokens_revocation CHECK ((revoked_at IS NULL) = (revoked_reason IS NULL)),
  CONSTRAINT customer_refresh_tokens_expiry CHECK (expires_at <= absolute_expires_at)
);

CREATE INDEX ix_customer_refresh_tokens_family ON customer_refresh_tokens (family_id);

CREATE INDEX ix_customer_refresh_tokens_customer ON customer_refresh_tokens (customer_id);

CREATE INDEX ix_customer_refresh_tokens_expires ON customer_refresh_tokens (expires_at);

CREATE TABLE account_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  purpose token_purpose NOT NULL,
  employee_id uuid REFERENCES employees (id),
  customer_id uuid REFERENCES customers (id),
  token_hash text NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  used_at timestamptz,
  revoked_at timestamptz,
  created_by uuid REFERENCES employees (id),
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT account_tokens_one_owner CHECK ((employee_id IS NULL) <> (customer_id IS NULL))
);

-- At most one open (unused, unrevoked) link per account and purpose: a resend must revoke the previous link.
CREATE UNIQUE INDEX ux_account_tokens_open_employee ON account_tokens (employee_id, purpose)
WHERE
  used_at IS NULL
  AND revoked_at IS NULL
  AND employee_id IS NOT NULL;

CREATE UNIQUE INDEX ux_account_tokens_open_customer ON account_tokens (customer_id, purpose)
WHERE
  used_at IS NULL
  AND revoked_at IS NULL
  AND customer_id IS NOT NULL;

-- Append-only; plain ids with no foreign keys so rows survive any deletion.
CREATE TABLE access_logs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  account_type account_type NOT NULL,
  employee_id uuid,
  customer_id uuid,
  attempted_document_type text,
  attempted_document_number text,
  action_type access_action NOT NULL,
  failure_reason access_failure_reason,
  device_info text,
  ip_address text,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT access_logs_one_owner_at_most CHECK (
    employee_id IS NULL
    OR customer_id IS NULL
  ),
  CONSTRAINT access_logs_owner_matches_account CHECK (
    (
      account_type = 'employee'
      AND customer_id IS NULL
    )
    OR (
      account_type = 'customer'
      AND employee_id IS NULL
    )
  )
);

CREATE INDEX ix_access_logs_created ON access_logs (created_at);

CREATE INDEX ix_access_logs_employee ON access_logs (employee_id, created_at);

CREATE INDEX ix_access_logs_customer ON access_logs (customer_id, created_at);

CREATE INDEX ix_access_logs_attempt ON access_logs (
  portal_type,
  attempted_document_type,
  attempted_document_number,
  created_at
);
