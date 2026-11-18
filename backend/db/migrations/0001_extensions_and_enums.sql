-- +goose Up
-- Extensions are created in the public schema so the immutable helper functions below can reference them.
-- (On Supabase, enable pg_trgm and unaccent with "schema: public" if they are not already enabled there.)
CREATE EXTENSION IF NOT EXISTS pg_trgm
WITH
  SCHEMA public;

CREATE EXTENSION IF NOT EXISTS unaccent
WITH
  SCHEMA public;

-- +goose StatementBegin
CREATE FUNCTION f_unaccent (text) RETURNS text LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT AS $$ SELECT public.unaccent('public.unaccent', $1) $$;

-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION f_search_key (text) RETURNS text LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT AS $$ SELECT lower(public.f_unaccent($1)) $$;

-- +goose StatementEnd
-- People and customers
CREATE TYPE employee_role AS ENUM('system_admin', 'sales_manager', 'sales_advisor');

CREATE TYPE employee_document_type AS ENUM('DNI', 'CE', 'PAS');

CREATE TYPE customer_document_type AS ENUM('DNI', 'CE', 'PAS', 'CDI', 'RUC');

CREATE TYPE customer_type AS ENUM(
  'diplomatico',
  'embajada',
  'entidad_religiosa_catolica',
  'entidad_religiosa_no_catolica',
  'organismo_internacional'
);

CREATE TYPE gender AS ENUM('M', 'F');

CREATE TYPE name_source AS ENUM('looked_up', 'manual');

-- Sales and tasks
CREATE TYPE sale_temperature AS ENUM('hot', 'warm', 'cold');

CREATE TYPE sale_state AS ENUM('active', 'completed', 'cancelled');

CREATE TYPE task_status AS ENUM('pending', 'in_progress', 'completed');

CREATE TYPE milestone_state AS ENUM('pending', 'in_progress', 'completed');

CREATE TYPE completion_phase AS ENUM(
  'early',
  'in_approaching',
  'overdue',
  'recurring_overdue',
  'not_applicable'
);

CREATE TYPE cancellation_category AS ENUM(
  'customer_withdrew',
  'payment_or_financing_failed',
  'unit_unavailable',
  'due_diligence_finding',
  'registration_error',
  'other'
);

CREATE TYPE pep_status AS ENUM('no_pep', 'pep', 'related_to_pep');

CREATE TYPE ddc_regime AS ENUM('general', 'reinforced');

CREATE TYPE control_channel AS ENUM('green', 'orange', 'red');

CREATE TYPE payment_kind AS ENUM('pana', 'toyota');

CREATE TYPE invoice_kind AS ENUM(
  'toyota',
  'commission',
  'accessories',
  'release_processing'
);

CREATE TYPE bank AS ENUM('bcp', 'bbva', 'scotiabank', 'interbank');

CREATE TYPE customs_office AS ENUM('callao_maritima', 'chancay');

-- Catalog
CREATE TYPE vehicle_category AS ENUM(
  'camioneta',
  'comercial',
  'familiar',
  'hatchback',
  'sedan'
);

CREATE TYPE viewpoint AS ENUM(
  'main_view',
  'exterior_front',
  'exterior_left',
  'exterior_right',
  'exterior_rear',
  'interior'
);

-- Documents
CREATE TYPE document_format AS ENUM('pdf', 'excel');

CREATE TYPE slot_requirement AS ENUM('always', 'conditional');

CREATE TYPE storage_deletion_reason AS ENUM(
  'document_replaced',
  'document_removed',
  'catalog_image_replaced',
  'catalog_image_removed',
  'employee_photo_replaced',
  'employee_photo_removed',
  'sale_deleted'
);

-- Authentication
CREATE TYPE token_purpose AS ENUM('activation', 'password_reset');

CREATE TYPE account_type AS ENUM('employee', 'customer');

-- Security events. Business events live in audit_logs; these record sign-in and credential events.
CREATE TYPE access_action AS ENUM(
  'login_success',
  'login_failed',
  'logout',
  'account_activated',
  'password_reset_requested',
  'password_reset_completed',
  'password_changed'
);

CREATE TYPE access_failure_reason AS ENUM(
  'invalid_credentials',
  'account_locked',
  'account_inactive',
  'not_activated'
);

CREATE TYPE revoked_reason AS ENUM(
  'logout',
  'reuse_detected',
  'password_reset',
  'deactivated'
);

-- Communication, lookups and audit
CREATE TYPE notification_status AS ENUM('pending', 'sent', 'failed', 'skipped');

CREATE TYPE notification_type AS ENUM(
  'employee_activation',
  'customer_activation',
  'password_reset',
  'sale_created',
  'milestone_task_1',
  'milestone_task_3',
  'milestone_task_6',
  'milestone_task_7',
  'milestone_task_10',
  'milestone_task_11',
  'milestone_task_13',
  'customer_advisor_changed', -- to the customer when the advisor on their sale changes
  'sale_assigned', -- to the advisor who receives a sale (created for them or reassigned)
  'sale_unassigned', -- to the previous advisor when a sale is reassigned
  'sla_warning',
  'sla_overdue_reminder'
);

CREATE TYPE lookup_kind AS ENUM('codart_dni', 'codart_ruc', 'customer_existence');

CREATE TYPE lookup_outcome AS ENUM(
  'success',
  'not_found',
  'error',
  'quota_blocked',
  'rate_limited'
);

CREATE TYPE actor_kind AS ENUM('employee', 'system');

CREATE TYPE audit_entity_type AS ENUM(
  'sale_register',
  'task_instance',
  'document',
  'customer',
  'employee',
  'vehicle_model',
  'vehicle_version',
  'vehicle_color',
  'catalog_image',
  'export'
);

-- Short verbs: the entity type (audit_entity_type) says what was acted on, and a CHECK on audit_logs
-- only allows the valid entity and verb pairs.
CREATE TYPE audit_action AS ENUM(
  'CREATED',
  'UPDATED',
  'DEACTIVATED',
  'REACTIVATED',
  'DELETED',
  'STARTED',
  'COMPLETED',
  'RESTARTED',
  'REOPENED',
  'CORRECTED',
  'CANCELLED',
  'REASSIGNED',
  'UPLOADED',
  'REPLACED',
  'REMOVED',
  'ROLE_CHANGED',
  'EXPORTED'
);
