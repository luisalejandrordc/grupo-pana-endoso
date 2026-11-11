-- +goose Up
-- Append-only; plain ids with no foreign keys so rows survive any deletion.
CREATE TABLE audit_logs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_employee_id uuid,
  actor_kind actor_kind NOT NULL,
  sale_id uuid,
  entity_type audit_entity_type NOT NULL,
  entity_id uuid NOT NULL,
  action audit_action NOT NULL,
  changes_summary jsonb,
  reason text,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT audit_logs_actor_matches_kind CHECK (
    (actor_kind = 'system') = (actor_employee_id IS NULL)
  ),
  CONSTRAINT audit_logs_valid_pair CHECK (
    (
      entity_type = 'sale_register'
      AND action IN (
        'CREATED',
        'UPDATED',
        'COMPLETED',
        'CANCELLED',
        'REOPENED',
        'REASSIGNED',
        'DELETED'
      )
    )
    OR (
      entity_type = 'task_instance'
      AND action IN (
        'STARTED',
        'UPDATED',
        'COMPLETED',
        'RESTARTED',
        'REOPENED',
        'CORRECTED'
      )
    )
    OR (
      entity_type = 'document'
      AND action IN ('UPLOADED', 'REPLACED', 'REMOVED')
    )
    OR (
      entity_type = 'customer'
      AND action IN (
        'CREATED',
        'UPDATED',
        'DEACTIVATED',
        'REACTIVATED',
        'DELETED'
      )
    )
    OR (
      entity_type = 'employee'
      AND action IN (
        'CREATED',
        'UPDATED',
        'ROLE_CHANGED',
        'DEACTIVATED',
        'REACTIVATED',
        'DELETED'
      )
    )
    OR (
      entity_type IN (
        'vehicle_model',
        'vehicle_version',
        'vehicle_color'
      )
      AND action IN (
        'CREATED',
        'UPDATED',
        'DEACTIVATED',
        'REACTIVATED',
        'DELETED'
      )
    )
    OR (
      entity_type = 'catalog_image'
      AND action IN ('UPLOADED', 'REPLACED', 'REMOVED')
    )
    OR (
      entity_type = 'export'
      AND action = 'EXPORTED'
    )
  ),
  CONSTRAINT audit_logs_reason_required CHECK (
    action NOT IN (
      'CANCELLED',
      'REOPENED',
      'RESTARTED',
      'CORRECTED',
      'DELETED'
    )
    OR length(btrim(coalesce(reason, ''))) > 0
  )
);

CREATE INDEX ix_audit_logs_sale ON audit_logs (sale_id, created_at);

CREATE INDEX ix_audit_logs_entity ON audit_logs (entity_type, entity_id, created_at);

CREATE INDEX ix_audit_logs_actor ON audit_logs (actor_employee_id, created_at);

CREATE INDEX ix_audit_logs_action ON audit_logs (action, created_at);

-- Transactional outbox for emails
CREATE TABLE notification_logs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  notification_type notification_type NOT NULL,
  sale_id uuid REFERENCES sale_registers (id) ON DELETE SET NULL,
  task_instance_id uuid REFERENCES task_instances (id) ON DELETE SET NULL,
  recipient_employee_id uuid REFERENCES employees (id),
  recipient_customer_id uuid REFERENCES customers (id),
  recipient_email text NOT NULL,
  dedupe_key text NOT NULL UNIQUE,
  status notification_status NOT NULL DEFAULT 'pending',
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  attempt_count integer NOT NULL DEFAULT 0,
  last_error text,
  created_at timestamptz NOT NULL DEFAULT now(),
  dispatched_at timestamptz,
  CONSTRAINT notification_logs_one_recipient_at_most CHECK (
    recipient_employee_id IS NULL
    OR recipient_customer_id IS NULL
  )
);

CREATE INDEX ix_notification_logs_status ON notification_logs (status, created_at);

CREATE INDEX ix_notification_logs_sale ON notification_logs (sale_id);

CREATE INDEX ix_notification_logs_due ON notification_logs (next_attempt_at)
WHERE
  status = 'pending';

-- Per-user rate limiting, existence-lookup log and the monthly Codart counter
CREATE TABLE lookup_logs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_id uuid NOT NULL REFERENCES employees (id),
  lookup_kind lookup_kind NOT NULL,
  document_type text,
  document_number text NOT NULL,
  outcome lookup_outcome NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ix_lookup_logs_kind ON lookup_logs (lookup_kind, created_at);

CREATE INDEX ix_lookup_logs_employee ON lookup_logs (employee_id, created_at);
