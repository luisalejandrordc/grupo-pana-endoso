-- +goose Up
CREATE TABLE sale_registers (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sale_number bigint GENERATED ALWAYS AS IDENTITY,
  customer_id uuid NOT NULL REFERENCES customers (id),
  -- Immutable snapshots taken at creation
  customer_legal_name_snapshot text NOT NULL,
  customer_doc_type_snapshot customer_document_type NOT NULL,
  customer_doc_number_snapshot text NOT NULL,
  customer_type_snapshot customer_type NOT NULL,
  vehicle_model_id uuid NOT NULL,
  vehicle_version_id uuid NOT NULL,
  vehicle_color_id uuid NOT NULL,
  vehicle_model_name_snapshot text NOT NULL,
  vehicle_version_name_snapshot text NOT NULL,
  vehicle_color_name_snapshot text NOT NULL,
  vehicle_reference_price_snapshot numeric(12, 2) NOT NULL CHECK (vehicle_reference_price_snapshot > 0),
  sale_temperature sale_temperature NOT NULL,
  -- State
  overall_state sale_state NOT NULL DEFAULT 'active',
  vehicle_series text,
  vin text,
  license_plate text,
  assigned_advisor_id uuid NOT NULL REFERENCES employees (id),
  sale_notes text,
  completed_at timestamptz,
  cancelled_at timestamptz,
  cancelled_by uuid REFERENCES employees (id),
  cancelled_at_task_number smallint CHECK (cancelled_at_task_number BETWEEN 1 AND 13),
  cancellation_category cancellation_category,
  cancellation_text text,
  row_version integer NOT NULL DEFAULT 1,
  created_by uuid NOT NULL REFERENCES employees (id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_by uuid REFERENCES employees (id),
  updated_at timestamptz NOT NULL DEFAULT now(),
  -- A version and a color must belong to the sale's model
  CONSTRAINT sale_registers_version_model_fk FOREIGN KEY (vehicle_version_id, vehicle_model_id) REFERENCES vehicle_versions (id, model_id),
  CONSTRAINT sale_registers_color_model_fk FOREIGN KEY (vehicle_color_id, vehicle_model_id) REFERENCES vehicle_colors (id, model_id),
  CONSTRAINT sale_registers_state_coherence CHECK (
    (
      overall_state = 'active'
      AND completed_at IS NULL
      AND cancelled_at IS NULL
      AND cancelled_by IS NULL
      AND cancelled_at_task_number IS NULL
      AND cancellation_category IS NULL
      AND cancellation_text IS NULL
    )
    OR (
      overall_state = 'completed'
      AND completed_at IS NOT NULL
      AND cancelled_at IS NULL
      AND cancelled_by IS NULL
      AND cancelled_at_task_number IS NULL
      AND cancellation_category IS NULL
      AND cancellation_text IS NULL
    )
    OR (
      overall_state = 'cancelled'
      AND completed_at IS NULL
      AND cancelled_at IS NOT NULL
      AND cancelled_by IS NOT NULL
      AND cancelled_at_task_number IS NOT NULL
      AND cancellation_category IS NOT NULL
    )
  ),
  CONSTRAINT sale_registers_other_needs_text CHECK (
    cancellation_category IS DISTINCT FROM 'other'
    OR length(btrim(coalesce(cancellation_text, ''))) > 0
  ),
  CONSTRAINT sale_registers_vin_format CHECK (
    vin IS NULL
    OR vin ~ '^[A-HJ-NPR-Z0-9]{17}$'
  ),
  CONSTRAINT sale_registers_plate_format CHECK (
    license_plate IS NULL
    OR license_plate ~ '^[A-Z0-9]{6}$'
  ),
  CONSTRAINT sale_registers_series_normalized CHECK (
    vehicle_series IS NULL
    OR (
      vehicle_series = upper(btrim(vehicle_series))
      AND vehicle_series !~ '\s'
      AND length(vehicle_series) > 0
    )
  )
);

CREATE UNIQUE INDEX ux_sale_registers_number ON sale_registers (sale_number);

-- Identifiers are unique only among active and completed sales; cancelled sales release them.
CREATE UNIQUE INDEX ux_sale_registers_series ON sale_registers (vehicle_series)
WHERE
  vehicle_series IS NOT NULL
  AND overall_state IN ('active', 'completed');

CREATE UNIQUE INDEX ux_sale_registers_vin ON sale_registers (vin)
WHERE
  vin IS NOT NULL
  AND overall_state IN ('active', 'completed');

CREATE UNIQUE INDEX ux_sale_registers_plate ON sale_registers (license_plate)
WHERE
  license_plate IS NOT NULL
  AND overall_state IN ('active', 'completed');

CREATE INDEX ix_sale_registers_advisor ON sale_registers (
  assigned_advisor_id,
  overall_state,
  created_at DESC
);

CREATE INDEX ix_sale_registers_customer ON sale_registers (customer_id);

CREATE INDEX ix_sale_registers_state ON sale_registers (overall_state, created_at DESC);

CREATE INDEX ix_sale_registers_model_version ON sale_registers (vehicle_model_id, vehicle_version_id);

CREATE INDEX ix_sale_registers_search_name ON sale_registers USING gin (
  f_search_key (customer_legal_name_snapshot) gin_trgm_ops
);

CREATE INDEX ix_sale_registers_search_series ON sale_registers USING gin (vehicle_series gin_trgm_ops);

CREATE INDEX ix_sale_registers_search_vin ON sale_registers USING gin (vin gin_trgm_ops);

CREATE INDEX ix_sale_registers_search_plate ON sale_registers USING gin (license_plate gin_trgm_ops);

CREATE TABLE task_instances (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sale_id uuid NOT NULL REFERENCES sale_registers (id) ON DELETE CASCADE,
  task_number smallint NOT NULL REFERENCES task_definitions (task_number),
  status task_status NOT NULL DEFAULT 'pending',
  -- When a task became eligible is not stored: it is derived (see v_task_eligibility).
  started_at timestamptz,
  completed_at timestamptz,
  due_date timestamptz,
  completion_phase completion_phase,
  notes text,
  approaching_warning_sent_at timestamptz,
  last_reminder_sent_at timestamptz,
  overdue_reminder_count smallint NOT NULL DEFAULT 0 CHECK (overdue_reminder_count BETWEEN 0 AND 6),
  row_version integer NOT NULL DEFAULT 1,
  created_by uuid NOT NULL REFERENCES employees (id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_by uuid REFERENCES employees (id),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT task_instances_sale_task_unique UNIQUE (sale_id, task_number),
  CONSTRAINT task_instances_id_sale_unique UNIQUE (id, sale_id),
  CONSTRAINT task_instances_status_coherence CHECK (
    (
      status = 'pending'
      AND started_at IS NULL
      AND completed_at IS NULL
      AND due_date IS NULL
      AND completion_phase IS NULL
    )
    OR (
      status = 'in_progress'
      AND started_at IS NOT NULL
      AND completed_at IS NULL
      AND completion_phase IS NULL
    )
    OR (
      status = 'completed'
      AND started_at IS NOT NULL
      AND completed_at IS NOT NULL
      AND completed_at >= started_at
      AND completion_phase IS NOT NULL
    )
  ),
  CONSTRAINT task_instances_reminders_need_due_date CHECK (
    overdue_reminder_count = 0
    OR due_date IS NOT NULL
  )
);

-- At most one task in progress per sale
CREATE UNIQUE INDEX ux_task_instances_one_in_progress ON task_instances (sale_id)
WHERE
  status = 'in_progress';

-- The scheduler's query
CREATE INDEX ix_task_instances_sla ON task_instances (status, due_date)
WHERE
  status = 'in_progress'
  AND due_date IS NOT NULL;

CREATE INDEX ix_task_instances_sale_status ON task_instances (sale_id, status);

-- Task 1 and Task 3 amounts
CREATE TABLE sale_financials (
  sale_id uuid PRIMARY KEY REFERENCES sale_registers (id) ON DELETE CASCADE,
  initial_toyota_amount numeric(12, 2) CHECK (initial_toyota_amount > 0),
  initial_commission numeric(12, 2) CHECK (initial_commission > 0),
  accessory_amount numeric(12, 2) CHECK (accessory_amount > 0),
  release_processing_amount numeric(12, 2) CHECK (release_processing_amount > 0),
  quoted_final_price numeric(12, 2) GENERATED ALWAYS AS (
    initial_toyota_amount + initial_commission + accessory_amount + release_processing_amount
  ) STORED,
  final_toyota_amount numeric(12, 2) CHECK (final_toyota_amount > 0),
  -- Generated columns cannot reference other generated columns, so the quoted price is expanded here.
  final_commission numeric(12, 2) GENERATED ALWAYS AS (
    (
      initial_toyota_amount + initial_commission + accessory_amount + release_processing_amount
    ) - final_toyota_amount - accessory_amount - release_processing_amount
  ) STORED,
  row_version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_by uuid REFERENCES employees (id),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT sale_financials_commission_positive CHECK (
    final_commission IS NULL
    OR final_commission > 0
  )
);

-- Task 2
CREATE TABLE sale_due_diligence (
  sale_id uuid PRIMARY KEY REFERENCES sale_registers (id) ON DELETE CASCADE,
  signer_document_type customer_document_type,
  signer_document_number text,
  signer_first_name text,
  signer_last_name text,
  signer_name_source name_source,
  pep_status pep_status,
  ddc_regime ddc_regime,
  row_version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_by uuid REFERENCES employees (id),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT sale_due_diligence_signer_pair CHECK (
    (signer_document_type IS NULL) = (signer_document_number IS NULL)
  ),
  CONSTRAINT sale_due_diligence_signer_not_ruc CHECK (
    signer_document_type IS NULL
    OR signer_document_type <> 'RUC'
  ),
  CONSTRAINT sale_due_diligence_signer_format CHECK (
    signer_document_number IS NULL
    OR (
      signer_document_type = 'DNI'
      AND signer_document_number ~ '^[0-9]{8}$'
    )
    OR (
      signer_document_type = 'CE'
      AND signer_document_number ~ '^[A-Z0-9]{9}$'
    )
    OR (
      signer_document_type = 'PAS'
      AND signer_document_number ~ '^[A-Z0-9]{6,12}$'
    )
    OR (
      signer_document_type = 'CDI'
      AND signer_document_number ~ '^[A-Z0-9]{8,12}$'
    )
  )
);

-- Tasks 4 and 5: at most one row per kind per sale
CREATE TABLE sale_payments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sale_id uuid NOT NULL REFERENCES sale_registers (id) ON DELETE CASCADE,
  payment_kind payment_kind NOT NULL,
  bank bank,
  payment_date date,
  transaction_number text,
  deposited_amount numeric(12, 2) CHECK (deposited_amount > 0),
  expected_amount numeric(12, 2) CHECK (expected_amount > 0),
  mismatch_reason text,
  row_version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_by uuid REFERENCES employees (id),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT sale_payments_sale_kind_unique UNIQUE (sale_id, payment_kind),
  -- Two separate uniqueness spaces (one per kind); never freed on cancellation
  CONSTRAINT sale_payments_transaction_unique UNIQUE (payment_kind, bank, transaction_number),
  CONSTRAINT sale_payments_transaction_normalized CHECK (
    transaction_number IS NULL
    OR (
      transaction_number = upper(transaction_number)
      AND transaction_number !~ '\s'
      AND length(transaction_number) > 0
    )
  ),
  CONSTRAINT sale_payments_mismatch_reason CHECK (
    deposited_amount IS NULL
    OR expected_amount IS NULL
    OR deposited_amount = expected_amount
    OR length(btrim(coalesce(mismatch_reason, ''))) > 0
  )
);

-- Task 6: at most one row per kind per sale
CREATE TABLE sale_invoices (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sale_id uuid NOT NULL REFERENCES sale_registers (id) ON DELETE CASCADE,
  invoice_kind invoice_kind NOT NULL,
  invoice_number text,
  invoice_date date,
  invoice_amount numeric(12, 2) CHECK (invoice_amount > 0),
  expected_amount numeric(12, 2) CHECK (expected_amount > 0),
  mismatch_reason text,
  row_version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_by uuid REFERENCES employees (id),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT sale_invoices_sale_kind_unique UNIQUE (sale_id, invoice_kind),
  CONSTRAINT sale_invoices_number_format CHECK (
    invoice_number IS NULL
    OR invoice_number ~ '^[A-Z0-9]{4}-[0-9]{1,8}$'
  ),
  CONSTRAINT sale_invoices_mismatch_reason CHECK (
    invoice_amount IS NULL
    OR expected_amount IS NULL
    OR invoice_amount = expected_amount
    OR length(btrim(coalesce(mismatch_reason, ''))) > 0
  )
);

-- Two uniqueness namespaces, both covering cancelled sales
CREATE UNIQUE INDEX ux_sale_invoices_number_toyota ON sale_invoices (invoice_number)
WHERE
  invoice_kind = 'toyota'
  AND invoice_number IS NOT NULL;

CREATE UNIQUE INDEX ux_sale_invoices_number_pana ON sale_invoices (invoice_number)
WHERE
  invoice_kind IN ('commission', 'accessories', 'release_processing')
  AND invoice_number IS NOT NULL;

-- Tasks 7 to 13 (scalar fields)
CREATE TABLE sale_fulfillment (
  sale_id uuid PRIMARY KEY REFERENCES sale_registers (id) ON DELETE CASCADE,
  dam_number text,
  customs_office customs_office,
  control_channel control_channel,
  release_person_in_charge text,
  registration_date date,
  pds_transfer_date date,
  equipment_service_order_number text,
  equipment_completion_date date,
  surquillo_transfer_date date,
  vehicle_delivery_date date,
  row_version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_by uuid REFERENCES employees (id),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT sale_fulfillment_dam_normalized CHECK (
    dam_number IS NULL
    OR (
      dam_number = upper(dam_number)
      AND dam_number !~ '\s'
      AND length(dam_number) > 0
    )
  )
);

-- DAM numbers are unique across all sales, cancelled ones included
CREATE UNIQUE INDEX ux_sale_fulfillment_dam ON sale_fulfillment (dam_number)
WHERE
  dam_number IS NOT NULL;
