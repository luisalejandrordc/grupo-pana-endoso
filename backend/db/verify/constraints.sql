-- EndosoFlow database checks. Run with: make db-verify
-- Everything happens inside one transaction that is rolled back at the end, so nothing is kept.
-- Each check prints PASS, or stops the script with FAIL and the reason.
\set ON_ERROR_STOP on
SET
  client_min_messages = notice;

\echo 'EndosoFlow database checks'
BEGIN;

-- Helpers ---------------------------------------------------------------------------------------
CREATE FUNCTION pg_temp.expect_ok (
  p_label text,
  p_sql text,
  p_check boolean DEFAULT false
) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  BEGIN
    EXECUTE p_sql;
    IF p_check THEN
      SET CONSTRAINTS ALL IMMEDIATE;
      SET CONSTRAINTS ALL DEFERRED;
    END IF;
  EXCEPTION WHEN OTHERS THEN
    RAISE EXCEPTION 'FAIL: % (unexpected %: %)', p_label, SQLSTATE, SQLERRM;
  END;
  RAISE NOTICE 'PASS: %', p_label;
END $$;

CREATE FUNCTION pg_temp.expect_fail (
  p_label text,
  p_sql text,
  p_state text,
  p_check boolean DEFAULT false
) RETURNS void LANGUAGE plpgsql AS $$
DECLARE
  v_failed boolean := false;
  v_state text;
BEGIN
  BEGIN
    EXECUTE p_sql;
    IF p_check THEN
      SET CONSTRAINTS ALL IMMEDIATE;
    END IF;
  EXCEPTION WHEN OTHERS THEN
    v_failed := true;
    v_state := SQLSTATE;
  END;
  IF NOT v_failed THEN
    IF p_check THEN
      SET CONSTRAINTS ALL DEFERRED;
    END IF;
    RAISE EXCEPTION 'FAIL: % (the statement succeeded but should have failed)', p_label;
  END IF;
  IF v_state <> p_state THEN
    RAISE EXCEPTION 'FAIL: % (expected SQLSTATE %, got %)', p_label, p_state, v_state;
  END IF;
  RAISE NOTICE 'PASS: %', p_label;
END $$;

CREATE FUNCTION pg_temp.expect_equal (p_label text, p_sql text, p_expected numeric) RETURNS void LANGUAGE plpgsql AS $$
DECLARE
  v numeric;
BEGIN
  EXECUTE p_sql INTO v;
  IF v IS DISTINCT FROM p_expected THEN
    RAISE EXCEPTION 'FAIL: % (expected %, got %)', p_label, p_expected, v;
  END IF;
  RAISE NOTICE 'PASS: %', p_label;
END $$;

CREATE FUNCTION pg_temp.make_sale (
  p_id uuid,
  p_series text,
  p_vin text,
  p_plate text,
  p_color uuid DEFAULT 'b2000000-0000-0000-0000-000000000001',
  p_children boolean DEFAULT true
) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO sale_registers (
    id, customer_id, customer_legal_name_snapshot, customer_doc_type_snapshot, customer_doc_number_snapshot,
    customer_type_snapshot, vehicle_model_id, vehicle_version_id, vehicle_color_id,
    vehicle_model_name_snapshot, vehicle_version_name_snapshot, vehicle_color_name_snapshot,
    vehicle_reference_price_snapshot, sale_temperature, vehicle_series, vin, license_plate,
    assigned_advisor_id, created_by)
  VALUES (
    p_id, 'c0000000-0000-0000-0000-000000000001', 'Ana Diplomatica', 'CE', 'CE1234567', 'diplomatico',
    'b0000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001', p_color,
    'Hilux', 'SR', 'Blanco', 35000.00, 'hot', p_series, p_vin, p_plate,
    'a0000000-0000-0000-0000-000000000002', 'a0000000-0000-0000-0000-000000000001');
  IF p_children THEN
    INSERT INTO task_instances (sale_id, task_number, created_by)
    SELECT p_id, n, 'a0000000-0000-0000-0000-000000000001'
    FROM generate_series(1, 13) AS n;
    INSERT INTO sale_financials (sale_id) VALUES (p_id);
    INSERT INTO sale_due_diligence (sale_id) VALUES (p_id);
    INSERT INTO sale_fulfillment (sale_id) VALUES (p_id);
  END IF;
END $$;

-- Seeded reference data (from migration 0012)
SELECT
  pg_temp.expect_equal (
    '6 milestones are seeded',
    'SELECT count(*) FROM milestone_definitions',
    6
  );

SELECT
  pg_temp.expect_equal (
    '13 tasks are seeded',
    'SELECT count(*) FROM task_definitions',
    13
  );

SELECT
  pg_temp.expect_equal (
    '6 tasks have an SLA',
    'SELECT count(*) FROM task_definitions WHERE recommended_duration_hours IS NOT NULL',
    6
  );

SELECT
  pg_temp.expect_equal (
    '19 document slots are seeded',
    'SELECT count(*) FROM document_slots',
    19
  );

SELECT
  pg_temp.expect_equal (
    '11 slots are customer-visible',
    'SELECT count(*) FROM document_slots WHERE is_customer_visible',
    11
  );

SELECT
  pg_temp.expect_equal (
    'only the power of attorney is conditional',
    $q$SELECT count(*) FROM document_slots WHERE requirement = 'conditional'$q$,
    1
  );

-- Fixtures (reference rows already exist after migration 0012; ON CONFLICT keeps the insert harmless)
INSERT INTO
  milestone_definitions (milestone_number, name_es, description_es)
SELECT
  n,
  'Hito ' || n,
  'Descripcion de prueba'
FROM
  generate_series(1, 6) AS n
ON CONFLICT DO NOTHING;

INSERT INTO
  task_definitions (
    task_number,
    name_es,
    description_es,
    milestone_number,
    recommended_duration_hours,
    reminder_interval_hours
  )
VALUES
  (1, 'T1', 'd', 1, NULL, NULL),
  (2, 'T2', 'd', 1, 72, 12),
  (3, 'T3', 'd', 2, 288, 24),
  (4, 'T4', 'd', 2, NULL, NULL),
  (5, 'T5', 'd', 3, NULL, NULL),
  (6, 'T6', 'd', 3, NULL, NULL),
  (7, 'T7', 'd', 4, 240, 24),
  (8, 'T8', 'd', 4, 288, 24),
  (9, 'T9', 'd', 5, 720, 48),
  (10, 'T10', 'd', 5, 360, 24),
  (11, 'T11', 'd', 6, NULL, NULL),
  (12, 'T12', 'd', 6, NULL, NULL),
  (13, 'T13', 'd', 6, NULL, NULL)
ON CONFLICT DO NOTHING;

INSERT INTO
  document_slots (
    slot_code,
    task_number,
    label_es,
    allowed_format,
    is_customer_visible,
    requirement
  )
VALUES
  ('test_slot', 1, 'Prueba', 'pdf', true, 'always')
ON CONFLICT DO NOTHING;

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
    created_by
  )
VALUES
  (
    'a0000000-0000-0000-0000-000000000001',
    'DNI',
    '10000001',
    'Ana',
    'Admin',
    'Peruana',
    'F',
    DATE '1990-01-01',
    DATE '2020-01-01',
    '999000001',
    'admin@test.pe',
    'system_admin',
    '00000000-0000-0000-0000-000000000001'
  ),
  (
    'a0000000-0000-0000-0000-000000000002',
    'DNI',
    '10000002',
    'Ado',
    'Advisor',
    'Peruana',
    'M',
    DATE '1991-01-01',
    DATE '2021-01-01',
    '999000002',
    'advisor@test.pe',
    'sales_advisor',
    '00000000-0000-0000-0000-000000000001'
  );

INSERT INTO
  customers (
    id,
    customer_type,
    document_type,
    document_number,
    first_name,
    last_name,
    gender,
    nationality,
    phone,
    email,
    created_by
  )
VALUES
  (
    'c0000000-0000-0000-0000-000000000001',
    'diplomatico',
    'CE',
    'CE1234567',
    'Ana',
    'Diplomatica',
    'F',
    'Francesa',
    '999111222',
    'ana@test.pe',
    'a0000000-0000-0000-0000-000000000001'
  );

-- Model 1 is active and complete (category, version, color, Main view image), all in one transaction.
INSERT INTO
  vehicle_models (id, model_name, created_by)
VALUES
  (
    'b0000000-0000-0000-0000-000000000001',
    'Hilux',
    'a0000000-0000-0000-0000-000000000001'
  );

INSERT INTO
  vehicle_model_categories (model_id, category)
VALUES
  (
    'b0000000-0000-0000-0000-000000000001',
    'camioneta'
  );

INSERT INTO
  vehicle_versions (
    id,
    model_id,
    version_name,
    reference_price,
    created_by
  )
VALUES
  (
    'b1000000-0000-0000-0000-000000000001',
    'b0000000-0000-0000-0000-000000000001',
    'SR',
    35000.00,
    'a0000000-0000-0000-0000-000000000001'
  );

INSERT INTO
  vehicle_colors (id, model_id, color_name, created_by)
VALUES
  (
    'b2000000-0000-0000-0000-000000000001',
    'b0000000-0000-0000-0000-000000000001',
    'Blanco',
    'a0000000-0000-0000-0000-000000000001'
  );

INSERT INTO
  catalog_images (
    id,
    vehicle_color_id,
    viewpoint,
    mime_type,
    size_bytes,
    storage_key,
    uploaded_by
  )
VALUES
  (
    'b3000000-0000-0000-0000-000000000001',
    'b2000000-0000-0000-0000-000000000001',
    'main_view',
    'image/jpeg',
    1000,
    'img-1',
    'a0000000-0000-0000-0000-000000000001'
  );

-- Model 2 is archived (so the model completeness rules skip it). Its color still needs a Main view image,
-- because that rule applies to every color. It serves the model-mismatch and selectable-color checks.
INSERT INTO
  vehicle_models (
    id,
    model_name,
    is_active,
    deactivated_at,
    created_by
  )
VALUES
  (
    'b0000000-0000-0000-0000-000000000002',
    'Yaris',
    false,
    now(),
    'a0000000-0000-0000-0000-000000000001'
  );

INSERT INTO
  vehicle_versions (
    id,
    model_id,
    version_name,
    reference_price,
    created_by
  )
VALUES
  (
    'b1000000-0000-0000-0000-000000000002',
    'b0000000-0000-0000-0000-000000000002',
    'XS',
    20000.00,
    'a0000000-0000-0000-0000-000000000001'
  );

INSERT INTO
  vehicle_colors (id, model_id, color_name, created_by)
VALUES
  (
    'b2000000-0000-0000-0000-000000000002',
    'b0000000-0000-0000-0000-000000000002',
    'Rojo',
    'a0000000-0000-0000-0000-000000000001'
  );

INSERT INTO
  catalog_images (
    id,
    vehicle_color_id,
    viewpoint,
    mime_type,
    size_bytes,
    storage_key,
    uploaded_by
  )
VALUES
  (
    'b3000000-0000-0000-0000-000000000002',
    'b2000000-0000-0000-0000-000000000002',
    'main_view',
    'image/jpeg',
    1000,
    'img-2',
    'a0000000-0000-0000-0000-000000000001'
  );

SELECT
  pg_temp.expect_equal (
    'employees default to a manually entered name',
    $q$SELECT count(*) FROM employees WHERE name_source = 'manual'$q$,
    3
  );

SELECT
  pg_temp.expect_ok (
    'fixtures satisfy the commit-time rules',
    'SELECT 1',
    true
  );

-- Sales -----------------------------------------------------------------------------------------
SELECT
  pg_temp.expect_ok (
    'sale with 13 tasks and its detail rows',
    $q$SELECT pg_temp.make_sale('d0000000-0000-0000-0000-000000000001', 'SERIE1', 'JTFBT3FJ0K1234567', 'ABC123')$q$,
    true
  );

SELECT
  pg_temp.expect_fail (
    'a sale without its tasks is rejected at commit',
    $q$SELECT pg_temp.make_sale('d0000000-0000-0000-0000-000000000009', 'SERIE9', 'JTFBT3FJ9K1234567', 'ZZZ999',
       'b2000000-0000-0000-0000-000000000001', false)$q$,
    'EF003',
    true
  );

SELECT
  pg_temp.expect_fail (
    'temperature is immutable',
    $q$UPDATE sale_registers SET sale_temperature = 'cold' WHERE id = 'd0000000-0000-0000-0000-000000000001'$q$,
    'EF001'
  );

SELECT
  pg_temp.expect_fail (
    'a VIN with the letter O is rejected',
    $q$SELECT pg_temp.make_sale('d0000000-0000-0000-0000-000000000008', 'SERIE8', 'JTFBT3FJOK1234567', 'QWE123')$q$,
    '23514'
  );

SELECT
  pg_temp.expect_fail (
    'a malformed plate is rejected',
    $q$SELECT pg_temp.make_sale('d0000000-0000-0000-0000-000000000007', 'SERIE7', 'JTFBT3FJ7K1234567', 'AB-123')$q$,
    '23514'
  );

SELECT
  pg_temp.expect_fail (
    'a color from another model is rejected',
    $q$SELECT pg_temp.make_sale('d0000000-0000-0000-0000-000000000004', 'SERIE4', 'JTFBT3FJ4K1234567', 'JKL012',
       'b2000000-0000-0000-0000-000000000002')$q$,
    '23503'
  );

SELECT
  pg_temp.expect_fail (
    'a duplicate series among active sales is rejected',
    $q$SELECT pg_temp.make_sale('d0000000-0000-0000-0000-000000000005', 'SERIE1', 'JTFBT3FJ5K1234567', 'MNO345')$q$,
    '23505'
  );

-- Tasks -----------------------------------------------------------------------------------------
SELECT
  pg_temp.expect_ok (
    'start task 1',
    $q$UPDATE task_instances SET status = 'in_progress', started_at = now() - interval '2 hours'
     WHERE sale_id = 'd0000000-0000-0000-0000-000000000001' AND task_number = 1$q$,
    true
  );

SELECT
  pg_temp.expect_fail (
    'only one task can be in progress',
    $q$UPDATE task_instances SET status = 'in_progress', started_at = now()
     WHERE sale_id = 'd0000000-0000-0000-0000-000000000001' AND task_number = 2$q$,
    '23505'
  );

SELECT
  pg_temp.expect_fail (
    'task 3 cannot complete before tasks 1 and 2',
    $q$UPDATE task_instances SET status = 'completed', started_at = now() - interval '1 hour',
            completed_at = now(), completion_phase = 'not_applicable'
     WHERE sale_id = 'd0000000-0000-0000-0000-000000000001' AND task_number = 3$q$,
    'EF002',
    true
  );

SELECT
  pg_temp.expect_fail (
    'a pending task cannot have a start time',
    $q$UPDATE task_instances SET started_at = now()
     WHERE sale_id = 'd0000000-0000-0000-0000-000000000001' AND task_number = 5$q$,
    '23514'
  );

SELECT
  pg_temp.expect_ok (
    'complete task 1',
    $q$UPDATE task_instances SET status = 'completed', completed_at = now(), completion_phase = 'not_applicable'
     WHERE sale_id = 'd0000000-0000-0000-0000-000000000001' AND task_number = 1$q$,
    true
  );

-- Money -----------------------------------------------------------------------------------------
SELECT
  pg_temp.expect_ok (
    'save the four Task 1 amounts',
    $q$UPDATE sale_financials SET initial_toyota_amount = 30000, initial_commission = 1500,
            accessory_amount = 500, release_processing_amount = 200
     WHERE sale_id = 'd0000000-0000-0000-0000-000000000001'$q$
  );

SELECT
  pg_temp.expect_equal (
    'quoted price is the sum of the four amounts',
    $q$SELECT quoted_final_price FROM sale_financials WHERE sale_id = 'd0000000-0000-0000-0000-000000000001'$q$,
    32200
  );

SELECT
  pg_temp.expect_ok (
    'save the final Toyota amount',
    $q$UPDATE sale_financials SET final_toyota_amount = 30100 WHERE sale_id = 'd0000000-0000-0000-0000-000000000001'$q$
  );

SELECT
  pg_temp.expect_equal (
    'final commission is calculated',
    $q$SELECT final_commission FROM sale_financials WHERE sale_id = 'd0000000-0000-0000-0000-000000000001'$q$,
    1400
  );

SELECT
  pg_temp.expect_fail (
    'final commission must stay positive',
    $q$UPDATE sale_financials SET final_toyota_amount = 33000 WHERE sale_id = 'd0000000-0000-0000-0000-000000000001'$q$,
    '23514'
  );

SELECT
  pg_temp.expect_fail (
    'zero amounts are rejected',
    $q$UPDATE sale_financials SET accessory_amount = 0 WHERE sale_id = 'd0000000-0000-0000-0000-000000000001'$q$,
    '23514'
  );

SELECT
  pg_temp.expect_ok (
    'second sale',
    $q$SELECT pg_temp.make_sale('d0000000-0000-0000-0000-000000000002', 'SERIE2', 'JTFBT3FJ1K1234567', 'DEF456')$q$,
    true
  );

-- Payments --------------------------------------------------------------------------------------
SELECT
  pg_temp.expect_ok (
    'Grupo Pana payment',
    $q$INSERT INTO sale_payments (sale_id, payment_kind, bank, payment_date, transaction_number, deposited_amount, expected_amount)
     VALUES ('d0000000-0000-0000-0000-000000000001', 'pana', 'bcp', DATE '2026-01-10', 'TX1', 32200, 32200)$q$
  );

SELECT
  pg_temp.expect_fail (
    'same bank and transaction number inside one uniqueness space',
    $q$INSERT INTO sale_payments (sale_id, payment_kind, bank, payment_date, transaction_number, deposited_amount, expected_amount)
     VALUES ('d0000000-0000-0000-0000-000000000002', 'pana', 'bcp', DATE '2026-01-11', 'TX1', 100, 100)$q$,
    '23505'
  );

SELECT
  pg_temp.expect_ok (
    'same bank and number in the other uniqueness space',
    $q$INSERT INTO sale_payments (sale_id, payment_kind, bank, payment_date, transaction_number, deposited_amount, expected_amount)
     VALUES ('d0000000-0000-0000-0000-000000000002', 'toyota', 'bcp', DATE '2026-01-11', 'TX1', 100, 100)$q$
  );

SELECT
  pg_temp.expect_fail (
    'an amount mismatch needs a reason',
    $q$INSERT INTO sale_payments (sale_id, payment_kind, bank, payment_date, transaction_number, deposited_amount, expected_amount)
     VALUES ('d0000000-0000-0000-0000-000000000002', 'pana', 'bbva', DATE '2026-01-12', 'TX2', 100, 90)$q$,
    '23514'
  );

SELECT
  pg_temp.expect_ok (
    'an amount mismatch with a reason',
    $q$INSERT INTO sale_payments (sale_id, payment_kind, bank, payment_date, transaction_number, deposited_amount, expected_amount, mismatch_reason)
     VALUES ('d0000000-0000-0000-0000-000000000002', 'pana', 'bbva', DATE '2026-01-12', 'TX2', 100, 90, 'Cargo bancario')$q$
  );

-- Invoices: two uniqueness namespaces -----------------------------------------------------------
SELECT
  pg_temp.expect_ok (
    'Toyota invoice number',
    $q$INSERT INTO sale_invoices (sale_id, invoice_kind, invoice_number, invoice_date, invoice_amount, expected_amount)
     VALUES ('d0000000-0000-0000-0000-000000000001', 'toyota', 'F001-1', DATE '2026-01-20', 30100, 30100)$q$
  );

SELECT
  pg_temp.expect_ok (
    'a Grupo Pana invoice may share a Toyota invoice number',
    $q$INSERT INTO sale_invoices (sale_id, invoice_kind, invoice_number, invoice_date, invoice_amount, expected_amount)
     VALUES ('d0000000-0000-0000-0000-000000000001', 'commission', 'F001-1', DATE '2026-01-20', 1400, 1400)$q$
  );

SELECT
  pg_temp.expect_fail (
    'Grupo Pana namespace: accessories cannot reuse the commission number',
    $q$INSERT INTO sale_invoices (sale_id, invoice_kind, invoice_number, invoice_date, invoice_amount, expected_amount)
     VALUES ('d0000000-0000-0000-0000-000000000001', 'accessories', 'F001-1', DATE '2026-01-20', 500, 500)$q$,
    '23505'
  );

SELECT
  pg_temp.expect_fail (
    'Toyota namespace: another Toyota invoice cannot reuse the number',
    $q$INSERT INTO sale_invoices (sale_id, invoice_kind, invoice_number, invoice_date, invoice_amount, expected_amount)
     VALUES ('d0000000-0000-0000-0000-000000000002', 'toyota', 'F001-1', DATE '2026-01-21', 100, 100)$q$,
    '23505'
  );

SELECT
  pg_temp.expect_fail (
    'a malformed invoice number is rejected',
    $q$INSERT INTO sale_invoices (sale_id, invoice_kind, invoice_number, invoice_date, invoice_amount, expected_amount)
     VALUES ('d0000000-0000-0000-0000-000000000001', 'release_processing', 'F1-1', DATE '2026-01-20', 200, 200)$q$,
    '23514'
  );

-- DAM number --------------------------------------------------------------------------------------
SELECT
  pg_temp.expect_ok (
    'DAM number',
    $q$UPDATE sale_fulfillment SET dam_number = 'DAM123456' WHERE sale_id = 'd0000000-0000-0000-0000-000000000001'$q$
  );

SELECT
  pg_temp.expect_fail (
    'DAM number is unique across sales',
    $q$UPDATE sale_fulfillment SET dam_number = 'DAM123456' WHERE sale_id = 'd0000000-0000-0000-0000-000000000002'$q$,
    '23505'
  );

-- Cancellation and identifiers --------------------------------------------------------------------
SELECT
  pg_temp.expect_ok (
    'cancel sale 1',
    $q$UPDATE sale_registers SET overall_state = 'cancelled', cancelled_at = now(),
            cancelled_by = 'a0000000-0000-0000-0000-000000000001', cancelled_at_task_number = 2,
            cancellation_category = 'customer_withdrew'
     WHERE id = 'd0000000-0000-0000-0000-000000000001'$q$
  );

SELECT
  pg_temp.expect_ok (
    'series, VIN and plate are free after cancellation',
    $q$SELECT pg_temp.make_sale('d0000000-0000-0000-0000-000000000003', 'SERIE1', 'JTFBT3FJ0K1234567', 'ABC123')$q$,
    true
  );

SELECT
  pg_temp.expect_fail (
    'reopening conflicts with the new sale holding those identifiers',
    $q$UPDATE sale_registers SET overall_state = 'active', cancelled_at = NULL, cancelled_by = NULL,
            cancelled_at_task_number = NULL, cancellation_category = NULL
     WHERE id = 'd0000000-0000-0000-0000-000000000001'$q$,
    '23505'
  );

SELECT
  pg_temp.expect_fail (
    'category other needs a text',
    $q$UPDATE sale_registers SET cancellation_category = 'other', cancellation_text = NULL
     WHERE id = 'd0000000-0000-0000-0000-000000000001'$q$,
    '23514'
  );

SELECT
  pg_temp.expect_fail (
    'invoice numbers stay reserved after cancellation',
    $q$INSERT INTO sale_invoices (sale_id, invoice_kind, invoice_number, invoice_date, invoice_amount, expected_amount)
     VALUES ('d0000000-0000-0000-0000-000000000003', 'toyota', 'F001-1', DATE '2026-01-22', 100, 100)$q$,
    '23505'
  );

-- Customers ---------------------------------------------------------------------------------------
SELECT
  pg_temp.expect_fail (
    'a B2B customer must use a RUC',
    $q$INSERT INTO customers (customer_type, document_type, document_number, legal_name, contact_first_name,
       contact_last_name, contact_job_title, nationality, phone, email, created_by)
     VALUES ('embajada', 'DNI', '12345678', 'Embajada X', 'A', 'B', 'C', 'Peruana', '1', 'x@x.pe',
             'a0000000-0000-0000-0000-000000000001')$q$,
    '23514'
  );

SELECT
  pg_temp.expect_fail (
    'a diplomatic customer cannot have a legal name',
    $q$INSERT INTO customers (customer_type, document_type, document_number, first_name, last_name, gender,
       legal_name, nationality, phone, email, created_by)
     VALUES ('diplomatico', 'CE', 'CE7654321', 'A', 'B', 'M', 'X', 'Peruana', '1', 'y@x.pe',
             'a0000000-0000-0000-0000-000000000001')$q$,
    '23514'
  );

SELECT
  pg_temp.expect_fail (
    'a malformed DNI is rejected',
    $q$INSERT INTO customers (customer_type, document_type, document_number, first_name, last_name, gender,
       nationality, phone, email, created_by)
     VALUES ('diplomatico', 'DNI', '1234', 'A', 'B', 'M', 'Peruana', '1', 'z@x.pe',
             'a0000000-0000-0000-0000-000000000001')$q$,
    '23514'
  );

-- Documents ---------------------------------------------------------------------------------------
SELECT
  pg_temp.expect_ok (
    'upload a document',
    $q$INSERT INTO documents (sale_id, task_instance_id, slot_code, original_file_name, mime_type, size_bytes,
       storage_key, checksum_sha256, uploaded_by)
     SELECT t.sale_id, t.id, 'test_slot', 'a.pdf', 'application/pdf', 1000, 'key-1', repeat('a', 64),
            'a0000000-0000-0000-0000-000000000001'
     FROM task_instances t WHERE t.sale_id = 'd0000000-0000-0000-0000-000000000003' AND t.task_number = 1$q$
  );

SELECT
  pg_temp.expect_fail (
    'a slot holds at most one file',
    $q$INSERT INTO documents (sale_id, task_instance_id, slot_code, original_file_name, mime_type, size_bytes,
       storage_key, checksum_sha256, uploaded_by)
     SELECT t.sale_id, t.id, 'test_slot', 'b.pdf', 'application/pdf', 1000, 'key-2', repeat('b', 64),
            'a0000000-0000-0000-0000-000000000001'
     FROM task_instances t WHERE t.sale_id = 'd0000000-0000-0000-0000-000000000003' AND t.task_number = 1$q$,
    '23505'
  );

SELECT
  pg_temp.expect_fail (
    'a file cannot point at another sale''s task',
    $q$INSERT INTO documents (sale_id, task_instance_id, slot_code, original_file_name, mime_type, size_bytes,
       storage_key, checksum_sha256, uploaded_by)
     SELECT 'd0000000-0000-0000-0000-000000000002'::uuid, t.id, 'test_slot', 'c.pdf', 'application/pdf', 1000,
            'key-3', repeat('c', 64), 'a0000000-0000-0000-0000-000000000001'
     FROM task_instances t WHERE t.sale_id = 'd0000000-0000-0000-0000-000000000003' AND t.task_number = 2$q$,
    '23503'
  );

SELECT
  pg_temp.expect_fail (
    'files over 10 MB are rejected',
    $q$INSERT INTO documents (sale_id, task_instance_id, slot_code, original_file_name, mime_type, size_bytes,
       storage_key, checksum_sha256, uploaded_by)
     SELECT t.sale_id, t.id, 'test_slot', 'd.pdf', 'application/pdf', 20000000, 'key-4', repeat('d', 64),
            'a0000000-0000-0000-0000-000000000001'
     FROM task_instances t WHERE t.sale_id = 'd0000000-0000-0000-0000-000000000002' AND t.task_number = 1$q$,
    '23514'
  );

-- Catalog -----------------------------------------------------------------------------------------
SELECT
  pg_temp.expect_fail (
    'a model without a version and a color is rejected at commit',
    $q$WITH m AS (
       INSERT INTO vehicle_models (id, model_name, created_by)
       VALUES ('b0000000-0000-0000-0000-000000000003', 'Corolla', 'a0000000-0000-0000-0000-000000000001')
       RETURNING id)
     INSERT INTO vehicle_model_categories (model_id, category) SELECT id, 'sedan' FROM m$q$,
    'EF005',
    true
  );

SELECT
  pg_temp.expect_fail (
    'a color needs its Main view image',
    $q$INSERT INTO vehicle_colors (id, model_id, color_name, created_by)
     VALUES ('b2000000-0000-0000-0000-000000000003', 'b0000000-0000-0000-0000-000000000001', 'Negro',
             'a0000000-0000-0000-0000-000000000001')$q$,
    'EF005',
    true
  );

SELECT
  pg_temp.expect_fail (
    'an archived color still needs its Main view image',
    $q$INSERT INTO vehicle_colors (id, model_id, color_name, is_active, deactivated_at, created_by)
     VALUES ('b2000000-0000-0000-0000-000000000004', 'b0000000-0000-0000-0000-000000000001', 'Gris',
             false, now(), 'a0000000-0000-0000-0000-000000000001')$q$,
    'EF005',
    true
  );

SELECT
  pg_temp.expect_fail (
    'the Main view image of a color cannot be deleted',
    $q$DELETE FROM catalog_images WHERE id = 'b3000000-0000-0000-0000-000000000001'$q$,
    'EF005',
    true
  );

SELECT
  pg_temp.expect_fail (
    'the last active version of an active model cannot be archived',
    $q$UPDATE vehicle_versions SET is_active = false, deactivated_at = now()
     WHERE id = 'b1000000-0000-0000-0000-000000000001'$q$,
    'EF005',
    true
  );

SELECT
  pg_temp.expect_fail (
    'a version price must be positive',
    $q$INSERT INTO vehicle_versions (model_id, version_name, reference_price, created_by)
     VALUES ('b0000000-0000-0000-0000-000000000001', 'Cero', 0, 'a0000000-0000-0000-0000-000000000001')$q$,
    '23514'
  );

SELECT
  pg_temp.expect_fail (
    'a failed-login count needs the start of its window',
    $q$UPDATE employees SET failed_login_count = 2 WHERE id = 'a0000000-0000-0000-0000-000000000002'$q$,
    '23514'
  );

SELECT
  pg_temp.expect_ok (
    'a failed-login count with its window start',
    $q$UPDATE employees SET failed_login_count = 2, first_failed_login_at = now()
     WHERE id = 'a0000000-0000-0000-0000-000000000002'$q$
  );

-- Staff -------------------------------------------------------------------------------------------
SELECT
  pg_temp.expect_fail (
    'the last active System Admin cannot be deactivated',
    $q$UPDATE employees SET is_active = false, deactivated_at = now()
     WHERE id = 'a0000000-0000-0000-0000-000000000001'$q$,
    'EF006',
    true
  );

-- Audit, access logs and tokens -------------------------------------------------------------------
SELECT
  pg_temp.expect_ok (
    'write an audit row',
    $q$INSERT INTO audit_logs (actor_employee_id, actor_kind, sale_id, entity_type, entity_id, action)
     VALUES ('a0000000-0000-0000-0000-000000000001', 'employee', 'd0000000-0000-0000-0000-000000000001',
             'sale_register', 'd0000000-0000-0000-0000-000000000001', 'CREATED')$q$
  );

SELECT
  pg_temp.expect_fail (
    'audit rows cannot be updated',
    $q$UPDATE audit_logs SET reason = 'x'$q$,
    'EF004'
  );

SELECT
  pg_temp.expect_fail (
    'audit rows cannot be deleted',
    $q$DELETE FROM audit_logs$q$,
    'EF004'
  );

SELECT
  pg_temp.expect_fail (
    'a cancellation audit row needs a reason',
    $q$INSERT INTO audit_logs (actor_employee_id, actor_kind, sale_id, entity_type, entity_id, action)
     VALUES ('a0000000-0000-0000-0000-000000000001', 'employee', 'd0000000-0000-0000-0000-000000000001',
             'sale_register', 'd0000000-0000-0000-0000-000000000001', 'CANCELLED')$q$,
    '23514'
  );

SELECT
  pg_temp.expect_fail (
    'a system actor has no employee id',
    $q$INSERT INTO audit_logs (actor_employee_id, actor_kind, entity_type, entity_id, action)
     VALUES ('a0000000-0000-0000-0000-000000000001', 'system', 'sale_register',
             'd0000000-0000-0000-0000-000000000001', 'COMPLETED')$q$,
    '23514'
  );

SELECT
  pg_temp.expect_fail (
    'an audit action must fit its entity type',
    $q$INSERT INTO audit_logs (actor_employee_id, actor_kind, entity_type, entity_id, action, reason)
     VALUES ('a0000000-0000-0000-0000-000000000001', 'employee', 'document',
             'd0000000-0000-0000-0000-000000000001', 'CANCELLED', 'motivo')$q$,
    '23514'
  );

SELECT
  pg_temp.expect_ok (
    'saving an in-progress task is audited as UPDATED',
    $q$INSERT INTO audit_logs (actor_employee_id, actor_kind, sale_id, entity_type, entity_id, action, changes_summary)
     VALUES ('a0000000-0000-0000-0000-000000000001', 'employee', 'd0000000-0000-0000-0000-000000000001',
             'task_instance', 'd0000000-0000-0000-0000-0000000000bb', 'UPDATED',
             '{"accessoryAmount": {"before": "500.00", "after": "550.00"}}')$q$
  );

SELECT
  pg_temp.expect_fail (
    'correcting a completed task needs a reason',
    $q$INSERT INTO audit_logs (actor_employee_id, actor_kind, sale_id, entity_type, entity_id, action)
     VALUES ('a0000000-0000-0000-0000-000000000001', 'employee', 'd0000000-0000-0000-0000-000000000001',
             'task_instance', 'd0000000-0000-0000-0000-0000000000bb', 'CORRECTED')$q$,
    '23514'
  );

SELECT
  pg_temp.expect_fail (
    'a permanent deletion needs a reason',
    $q$INSERT INTO audit_logs (actor_employee_id, actor_kind, entity_type, entity_id, action)
     VALUES ('a0000000-0000-0000-0000-000000000001', 'employee', 'customer',
             'c0000000-0000-0000-0000-000000000001', 'DELETED')$q$,
    '23514'
  );

SELECT
  pg_temp.expect_ok (
    'a permanent deletion with a reason',
    $q$INSERT INTO audit_logs (actor_employee_id, actor_kind, entity_type, entity_id, action, reason)
     VALUES ('a0000000-0000-0000-0000-000000000001', 'employee', 'customer',
             'c0000000-0000-0000-0000-000000000001', 'DELETED', 'Registro de prueba')$q$
  );

SELECT
  pg_temp.expect_ok (
    'an export is audited',
    $q$INSERT INTO audit_logs (actor_employee_id, actor_kind, entity_type, entity_id, action)
     VALUES ('a0000000-0000-0000-0000-000000000001', 'employee', 'export',
             'd0000000-0000-0000-0000-0000000000aa', 'EXPORTED')$q$
  );

SELECT
  pg_temp.expect_fail (
    'an access log cannot name two owners',
    $q$INSERT INTO access_logs (account_type, employee_id, customer_id, action_type)
     VALUES ('employee', 'a0000000-0000-0000-0000-000000000001',
             'c0000000-0000-0000-0000-000000000001', 'login_success')$q$,
    '23514'
  );

SELECT
  pg_temp.expect_ok (
    'open activation link',
    $q$INSERT INTO account_tokens (purpose, employee_id, token_hash, expires_at)
     VALUES ('activation', 'a0000000-0000-0000-0000-000000000002', 'h1', now() + interval '3 days')$q$
  );

SELECT
  pg_temp.expect_fail (
    'only one open activation link per account',
    $q$INSERT INTO account_tokens (purpose, employee_id, token_hash, expires_at)
     VALUES ('activation', 'a0000000-0000-0000-0000-000000000002', 'h2', now() + interval '3 days')$q$,
    '23505'
  );

SELECT
  pg_temp.expect_fail (
    'a token needs exactly one owner',
    $q$INSERT INTO account_tokens (purpose, token_hash, expires_at)
     VALUES ('activation', 'h3', now() + interval '3 days')$q$,
    '23514'
  );

-- Views -------------------------------------------------------------------------------------------
SELECT
  pg_temp.expect_equal (
    'the current task of sale 1 is task 2',
    $q$SELECT current_task_number FROM v_sale_current_task WHERE sale_id = 'd0000000-0000-0000-0000-000000000001'$q$,
    2
  );

SELECT
  pg_temp.expect_equal (
    'milestone 1 of sale 1 is in progress',
    $q$SELECT count(*) FROM v_sale_milestones
     WHERE sale_id = 'd0000000-0000-0000-0000-000000000001' AND milestone_number = 1 AND state = 'in_progress'$q$,
    1
  );

SELECT
  pg_temp.expect_equal (
    'milestone 2 of sale 1 is pending',
    $q$SELECT count(*) FROM v_sale_milestones
     WHERE sale_id = 'd0000000-0000-0000-0000-000000000001' AND milestone_number = 2 AND state = 'pending'$q$,
    1
  );

SELECT
  pg_temp.expect_equal (
    'task 1 is eligible from the sale creation',
    $q$SELECT count(*) FROM v_task_eligibility e JOIN sale_registers s ON s.id = e.sale_id
     WHERE e.sale_id = 'd0000000-0000-0000-0000-000000000001' AND e.task_number = 1 AND e.eligible_at = s.created_at$q$,
    1
  );

SELECT
  pg_temp.expect_equal (
    'task 2 became eligible when task 1 was completed',
    $q$SELECT count(*) FROM v_task_eligibility e JOIN task_instances prev
       ON prev.sale_id = e.sale_id AND prev.task_number = 1
     WHERE e.sale_id = 'd0000000-0000-0000-0000-000000000001' AND e.task_number = 2
       AND e.eligible_at = prev.completed_at$q$,
    1
  );

SELECT
  pg_temp.expect_equal (
    'task 3 is not eligible while task 2 is not completed',
    $q$SELECT count(*) FROM v_task_eligibility
     WHERE sale_id = 'd0000000-0000-0000-0000-000000000001' AND task_number = 3 AND eligible_at IS NOT NULL$q$,
    0
  );

SELECT
  pg_temp.expect_equal (
    'an active color with a Main view is selectable',
    $q$SELECT count(*) FROM v_selectable_colors WHERE color_id = 'b2000000-0000-0000-0000-000000000001'$q$,
    1
  );

SELECT
  pg_temp.expect_equal (
    'a color of an archived model is not selectable',
    $q$SELECT count(*) FROM v_selectable_colors WHERE color_id = 'b2000000-0000-0000-0000-000000000002'$q$,
    0
  );

SELECT
  pg_temp.expect_equal (
    'the five dashboard date indexes exist',
    $q$SELECT count(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname IN (
      'ix_sale_registers_completed_at', 'ix_sale_registers_cancelled_at', 'ix_task_instances_completed',
      'ix_sale_payments_date', 'ix_sale_invoices_date')$q$,
    5
  );

\echo 'ALL CHECKS PASSED'
ROLLBACK;
