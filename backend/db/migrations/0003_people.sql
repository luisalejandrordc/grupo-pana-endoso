-- +goose Up
CREATE TABLE employees (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  document_type employee_document_type NOT NULL,
  document_number text NOT NULL,
  first_name text NOT NULL,
  last_name text NOT NULL,
  name_source name_source NOT NULL DEFAULT 'manual',
  nationality text NOT NULL,
  gender gender NOT NULL,
  birthdate date NOT NULL,
  hire_date date NOT NULL,
  personal_phone text,
  personal_email text,
  business_phone text NOT NULL,
  business_email text NOT NULL,
  profile_picture_key text,
  profile_picture_mime text,
  profile_picture_size integer,
  role employee_role NOT NULL,
  password_hash text,
  activated_at timestamptz,
  failed_login_count integer NOT NULL DEFAULT 0,
  first_failed_login_at timestamptz, -- start of the current 15-minute failure window
  locked_until timestamptz,
  is_system boolean NOT NULL DEFAULT false,
  is_active boolean NOT NULL DEFAULT true,
  deactivated_at timestamptz,
  deactivated_by uuid REFERENCES employees (id),
  row_version integer NOT NULL DEFAULT 1,
  created_by uuid REFERENCES employees (id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_by uuid REFERENCES employees (id),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT employees_document_unique UNIQUE (document_type, document_number),
  CONSTRAINT employees_document_format CHECK (
    (
      document_type = 'DNI'
      AND document_number ~ '^[0-9]{8}$'
    )
    OR (
      document_type = 'CE'
      AND document_number ~ '^[A-Z0-9]{9}$'
    )
    OR (
      document_type = 'PAS'
      AND document_number ~ '^[A-Z0-9]{6,12}$'
    )
  ),
  CONSTRAINT employees_business_email_lower CHECK (business_email = lower(business_email)),
  CONSTRAINT employees_picture_all_or_none CHECK (
    (profile_picture_key IS NULL) = (profile_picture_mime IS NULL)
    AND (profile_picture_key IS NULL) = (profile_picture_size IS NULL)
  ),
  CONSTRAINT employees_system_has_no_credentials CHECK (
    NOT is_system
    OR password_hash IS NULL
  ),
  CONSTRAINT employees_lifecycle CHECK (is_active = (deactivated_at IS NULL)),
  CONSTRAINT employees_failed_logins_nonnegative CHECK (failed_login_count >= 0),
  CONSTRAINT employees_failed_logins_window CHECK (
    failed_login_count = 0
    OR first_failed_login_at IS NOT NULL
  )
);

CREATE UNIQUE INDEX ux_employees_business_email ON employees (business_email);

CREATE INDEX ix_employees_role_active ON employees (role, is_active);

-- The system actor owns seeded records and cannot log in (no credentials, inactive, flagged as system).
INSERT INTO
  employees (
    id,
    document_type,
    document_number,
    first_name,
    last_name,
    nationality,
    gender,
    birthdate,
    hire_date,
    business_phone,
    business_email,
    role,
    is_system,
    is_active,
    deactivated_at
  )
VALUES
  (
    '00000000-0000-0000-0000-000000000001',
    'DNI',
    '00000000',
    'System',
    'User',
    'Peruana',
    'M',
    DATE '2000-01-01',
    DATE '2000-01-01',
    '000000000',
    'system@endosoflow.com',
    'system_admin',
    true,
    false,
    now()
  );

CREATE TABLE customers (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_type customer_type NOT NULL,
  document_type customer_document_type NOT NULL,
  document_number text NOT NULL,
  first_name text,
  last_name text,
  gender gender,
  legal_name text,
  display_name text GENERATED ALWAYS AS (
    COALESCE(legal_name, first_name || ' ' || last_name)
  ) STORED,
  name_source name_source NOT NULL DEFAULT 'manual',
  nationality text NOT NULL,
  phone text NOT NULL,
  email text NOT NULL,
  contact_first_name text,
  contact_last_name text,
  contact_job_title text,
  password_hash text,
  activated_at timestamptz,
  failed_login_count integer NOT NULL DEFAULT 0,
  first_failed_login_at timestamptz, -- start of the current 15-minute failure window
  locked_until timestamptz,
  is_active boolean NOT NULL DEFAULT true,
  deactivated_at timestamptz,
  deactivated_by uuid REFERENCES employees (id),
  row_version integer NOT NULL DEFAULT 1,
  created_by uuid NOT NULL REFERENCES employees (id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_by uuid REFERENCES employees (id),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT customers_document_unique UNIQUE (document_type, document_number),
  CONSTRAINT customers_document_format CHECK (
    (
      document_type = 'DNI'
      AND document_number ~ '^[0-9]{8}$'
    )
    OR (
      document_type = 'RUC'
      AND document_number ~ '^[0-9]{11}$'
    )
    OR (
      document_type = 'CE'
      AND document_number ~ '^[A-Z0-9]{9}$'
    )
    OR (
      document_type = 'PAS'
      AND document_number ~ '^[A-Z0-9]{6,12}$'
    )
    OR (
      document_type = 'CDI'
      AND document_number ~ '^[A-Z0-9]{8,12}$'
    )
  ),
  CONSTRAINT customers_shape CHECK (
    (
      customer_type = 'diplomatico'
      AND first_name IS NOT NULL
      AND last_name IS NOT NULL
      AND gender IS NOT NULL
      AND legal_name IS NULL
      AND contact_first_name IS NULL
      AND contact_last_name IS NULL
      AND contact_job_title IS NULL
      AND document_type IN ('DNI', 'CE', 'PAS', 'CDI')
    )
    OR (
      customer_type <> 'diplomatico'
      AND legal_name IS NOT NULL
      AND contact_first_name IS NOT NULL
      AND contact_last_name IS NOT NULL
      AND contact_job_title IS NOT NULL
      AND first_name IS NULL
      AND last_name IS NULL
      AND gender IS NULL
      AND document_type = 'RUC'
    )
  ),
  CONSTRAINT customers_email_lower CHECK (email = lower(email)),
  CONSTRAINT customers_lifecycle CHECK (is_active = (deactivated_at IS NULL)),
  CONSTRAINT customers_failed_logins_nonnegative CHECK (failed_login_count >= 0),
  CONSTRAINT customers_failed_logins_window CHECK (
    failed_login_count = 0
    OR first_failed_login_at IS NOT NULL
  )
);

CREATE INDEX ix_customers_created_by ON customers (created_by);

CREATE INDEX ix_customers_active ON customers (is_active);

CREATE INDEX ix_customers_search ON customers USING gin (f_search_key (display_name) gin_trgm_ops);
