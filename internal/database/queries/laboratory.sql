-- ============================================================================
-- LAB TEST METHODS
-- ============================================================================

-- name: CreateLabTestMethod :one
INSERT INTO lab_test_methods (
    method_code, method_name, description,
    standard_reference, standard_organization,
    test_type, test_category, methodology,
    sample_size, sample_unit_id, preparation_time, test_duration,
    required_equipment, specification_limits,
    version, effective_date, supersedes_method_id,
    status, approved_by, approval_date,
    attachments, notes, is_active
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23
)
RETURNING *;

-- name: GetLabTestMethodByID :one
SELECT 
    ltm.*,
    u.abbreviation as sample_unit_abbreviation,
    app.full_name as approved_by_name
FROM lab_test_methods ltm
LEFT JOIN measure_units u ON ltm.sample_unit_id = u.id
LEFT JOIN users app ON ltm.approved_by = app.id
WHERE ltm.id = $1;

-- name: GetLabTestMethodByCode :one
SELECT * FROM lab_test_methods
WHERE method_code = $1;

-- name: ListLabTestMethods :many
SELECT 
    id, method_code, method_name, test_type, test_category,
    standard_reference, version, status, is_active
FROM lab_test_methods
WHERE is_active = TRUE
ORDER BY method_code
LIMIT $1 OFFSET $2;

-- name: ListLabTestMethodsByType :many
SELECT * FROM lab_test_methods
WHERE test_type = $1 AND is_active = TRUE
ORDER BY method_code;

-- name: ListLabTestMethodsByStatus :many
SELECT * FROM lab_test_methods
WHERE status = $1
ORDER BY method_code
LIMIT $2 OFFSET $3;

-- name: SearchLabTestMethods :many
SELECT * FROM lab_test_methods
WHERE (method_code ILIKE '%' || $1 || '%' OR method_name ILIKE '%' || $1 || '%')
    AND is_active = TRUE
ORDER BY method_code
LIMIT $2 OFFSET $3;

-- name: UpdateLabTestMethod :one
UPDATE lab_test_methods
SET 
    method_name = COALESCE(sqlc.narg('method_name'), method_name),
    description = COALESCE(sqlc.narg('description'), description),
    standard_reference = COALESCE(sqlc.narg('standard_reference'), standard_reference),
    methodology = COALESCE(sqlc.narg('methodology'), methodology),
    status = COALESCE(sqlc.narg('status'), status),
    approved_by = COALESCE(sqlc.narg('approved_by'), approved_by),
    approval_date = COALESCE(sqlc.narg('approval_date'), approval_date),
    is_active = COALESCE(sqlc.narg('is_active'), is_active),
    notes = COALESCE(sqlc.narg('notes'), notes)
WHERE id = $1
RETURNING *;

-- name: DeleteLabTestMethod :exec
DELETE FROM lab_test_methods
WHERE id = $1;

-- ============================================================================
-- LAB EQUIPMENT
-- ============================================================================

-- name: CreateLabEquipment :one
INSERT INTO lab_equipment (
    equipment_code, equipment_name, equipment_type,
    manufacturer, model_number, serial_number,
    location, warehouse_id,
    calibration_frequency_days, last_calibration_date, next_calibration_date,
    calibration_status, calibration_certificate,
    last_maintenance_date, next_maintenance_date, maintenance_notes,
    is_operational, is_qualified, qualification_date,
    attachments, notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21
)
RETURNING *;

-- name: GetLabEquipmentByID :one
SELECT 
    le.*,
    w.name as warehouse_name
FROM lab_equipment le
LEFT JOIN warehouses w ON le.warehouse_id = w.id
WHERE le.id = $1;

-- name: GetLabEquipmentByCode :one
SELECT * FROM lab_equipment
WHERE equipment_code = $1;

-- name: ListLabEquipment :many
SELECT 
    id, equipment_code, equipment_name, equipment_type,
    location, calibration_status, next_calibration_date,
    is_operational, is_qualified
FROM lab_equipment
ORDER BY equipment_code
LIMIT $1 OFFSET $2;

-- name: ListLabEquipmentByType :many
SELECT * FROM lab_equipment
WHERE equipment_type = $1
ORDER BY equipment_code;

-- name: ListLabEquipmentByCalibrationStatus :many
SELECT * FROM lab_equipment
WHERE calibration_status = $1
ORDER BY next_calibration_date
LIMIT $2 OFFSET $3;

-- name: ListLabEquipmentCalibrationDue :many
SELECT * FROM lab_equipment
WHERE next_calibration_date <= $1
ORDER BY next_calibration_date;

-- name: ListOperationalLabEquipment :many
SELECT * FROM lab_equipment
WHERE is_operational = TRUE AND is_qualified = TRUE
ORDER BY equipment_code;

-- name: UpdateLabEquipment :one
UPDATE lab_equipment
SET 
    equipment_name = COALESCE(sqlc.narg('equipment_name'), equipment_name),
    location = COALESCE(sqlc.narg('location'), location),
    last_calibration_date = COALESCE(sqlc.narg('last_calibration_date'), last_calibration_date),
    next_calibration_date = COALESCE(sqlc.narg('next_calibration_date'), next_calibration_date),
    calibration_certificate = COALESCE(sqlc.narg('calibration_certificate'), calibration_certificate),
    last_maintenance_date = COALESCE(sqlc.narg('last_maintenance_date'), last_maintenance_date),
    next_maintenance_date = COALESCE(sqlc.narg('next_maintenance_date'), next_maintenance_date),
    maintenance_notes = COALESCE(sqlc.narg('maintenance_notes'), maintenance_notes),
    is_operational = COALESCE(sqlc.narg('is_operational'), is_operational),
    is_qualified = COALESCE(sqlc.narg('is_qualified'), is_qualified),
    notes = COALESCE(sqlc.narg('notes'), notes)
WHERE id = $1
RETURNING *;

-- name: DeleteLabEquipment :exec
DELETE FROM lab_equipment
WHERE id = $1;

-- ============================================================================
-- LAB SAMPLES
-- ============================================================================

-- name: CreateLabSample :one
INSERT INTO lab_samples (
    sample_number, sample_type, sample_status,
    material_id, batch_number, lot_number,
    quality_inspection_id, purchase_order_id, stock_transaction_id,
    sample_quantity, sample_unit_id, container_type, container_count,
    storage_location, storage_conditions,
    collected_by, collection_date, collection_method, sampling_plan,
    received_by_lab, lab_received_date,
    retention_required, retention_period_days, retention_expiry_date,
    is_external_lab, external_lab_name, external_lab_reference,
    sent_to_lab_date, expected_results_date,
    attachments, notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31
)
RETURNING *;

-- name: GetLabSampleByID :one
SELECT 
    ls.*,
    m.name as material_name,
    m.sku as material_sku,
    u.abbreviation as unit_abbreviation,
    collected.full_name as collected_by_name,
    received.full_name as received_by_lab_name
FROM lab_samples ls
LEFT JOIN materials m ON ls.material_id = m.id
LEFT JOIN measure_units u ON ls.sample_unit_id = u.id
LEFT JOIN users collected ON ls.collected_by = collected.id
LEFT JOIN users received ON ls.received_by_lab = received.id
WHERE ls.id = $1;

-- name: GetLabSampleByNumber :one
SELECT * FROM lab_samples
WHERE sample_number = $1;

-- name: ListLabSamples :many
SELECT 
    ls.id, ls.sample_number, ls.sample_type, ls.sample_status,
    ls.material_id, m.name as material_name, m.sku as material_sku,
    ls.batch_number, ls.sample_quantity, u.abbreviation as unit_abbreviation,
    ls.collection_date, ls.storage_location,
    ls.is_external_lab, ls.external_lab_name,
    ls.created_at
FROM lab_samples ls
LEFT JOIN materials m ON ls.material_id = m.id
LEFT JOIN measure_units u ON ls.sample_unit_id = u.id
ORDER BY ls.created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListLabSamplesByStatus :many
SELECT * FROM lab_samples
WHERE sample_status = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListLabSamplesByType :many
SELECT * FROM lab_samples
WHERE sample_type = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListLabSamplesByMaterial :many
SELECT * FROM lab_samples
WHERE material_id = $1
ORDER BY created_at DESC;

-- name: ListLabSamplesByBatch :many
SELECT * FROM lab_samples
WHERE batch_number = $1
ORDER BY created_at DESC;

-- name: ListLabSamplesPendingDisposal :many
SELECT * FROM lab_samples
WHERE retention_expiry_date <= $1
    AND sample_status IN ('retained', 'completed')
ORDER BY retention_expiry_date;

-- name: UpdateLabSample :one
UPDATE lab_samples
SET 
    sample_status = COALESCE(sqlc.narg('sample_status'), sample_status),
    received_by_lab = COALESCE(sqlc.narg('received_by_lab'), received_by_lab),
    lab_received_date = COALESCE(sqlc.narg('lab_received_date'), lab_received_date),
    storage_location = COALESCE(sqlc.narg('storage_location'), storage_location),
    disposed_date = COALESCE(sqlc.narg('disposed_date'), disposed_date),
    disposed_by = COALESCE(sqlc.narg('disposed_by'), disposed_by),
    disposal_method = COALESCE(sqlc.narg('disposal_method'), disposal_method),
    notes = COALESCE(sqlc.narg('notes'), notes)
WHERE id = $1
RETURNING *;

-- name: DeleteLabSample :exec
DELETE FROM lab_samples
WHERE id = $1;

-- ============================================================================
-- LAB TEST ASSIGNMENTS
-- ============================================================================

-- name: CreateLabTestAssignment :one
INSERT INTO lab_test_assignments (
    sample_id, test_method_id,
    priority, requested_date, scheduled_date, due_date,
    assigned_to, assigned_date,
    status, is_rush, notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
RETURNING *;

-- name: GetLabTestAssignmentByID :one
SELECT 
    lta.*,
    ls.sample_number,
    ls.material_id,
    m.name as material_name,
    ls.batch_number,
    ltm.method_code,
    ltm.method_name,
    assigned.full_name as assigned_to_name,
    reviewed.full_name as reviewed_by_name
FROM lab_test_assignments lta
LEFT JOIN lab_samples ls ON lta.sample_id = ls.id
LEFT JOIN materials m ON ls.material_id = m.id
LEFT JOIN lab_test_methods ltm ON lta.test_method_id = ltm.id
LEFT JOIN users assigned ON lta.assigned_to = assigned.id
LEFT JOIN users reviewed ON lta.reviewed_by = reviewed.id
WHERE lta.id = $1;

-- name: ListLabTestAssignments :many
SELECT 
    lta.id, lta.status, lta.priority, lta.is_rush,
    lta.scheduled_date, lta.due_date,
    ls.sample_number, ls.batch_number,
    m.name as material_name,
    ltm.method_code, ltm.method_name,
    assigned.full_name as assigned_to_name,
    lta.created_at
FROM lab_test_assignments lta
LEFT JOIN lab_samples ls ON lta.sample_id = ls.id
LEFT JOIN materials m ON ls.material_id = m.id
LEFT JOIN lab_test_methods ltm ON lta.test_method_id = ltm.id
LEFT JOIN users assigned ON lta.assigned_to = assigned.id
ORDER BY lta.created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListLabTestAssignmentsBySample :many
SELECT 
    lta.*,
    ltm.method_code,
    ltm.method_name,
    assigned.full_name as assigned_to_name
FROM lab_test_assignments lta
LEFT JOIN lab_test_methods ltm ON lta.test_method_id = ltm.id
LEFT JOIN users assigned ON lta.assigned_to = assigned.id
WHERE lta.sample_id = $1
ORDER BY lta.created_at;

-- name: ListLabTestAssignmentsByStatus :many
SELECT * FROM lab_test_assignments
WHERE status = $1
ORDER BY priority, due_date
LIMIT $2 OFFSET $3;

-- name: ListLabTestAssignmentsByAnalyst :many
SELECT * FROM lab_test_assignments
WHERE assigned_to = $1 AND status IN ('pending', 'in_progress')
ORDER BY is_rush DESC, priority, due_date
LIMIT $2 OFFSET $3;

-- name: ListPendingLabTestAssignments :many
SELECT * FROM lab_test_assignments
WHERE status IN ('pending', 'in_progress')
ORDER BY is_rush DESC, priority, due_date
LIMIT $1 OFFSET $2;

-- name: ListOverdueLabTestAssignments :many
SELECT * FROM lab_test_assignments
WHERE status IN ('pending', 'in_progress')
    AND due_date < CURRENT_DATE
ORDER BY due_date;

-- name: UpdateLabTestAssignment :one
UPDATE lab_test_assignments
SET 
    status = COALESCE(sqlc.narg('status'), status),
    assigned_to = COALESCE(sqlc.narg('assigned_to'), assigned_to),
    assigned_date = COALESCE(sqlc.narg('assigned_date'), assigned_date),
    scheduled_date = COALESCE(sqlc.narg('scheduled_date'), scheduled_date),
    due_date = COALESCE(sqlc.narg('due_date'), due_date),
    started_date = COALESCE(sqlc.narg('started_date'), started_date),
    completed_date = COALESCE(sqlc.narg('completed_date'), completed_date),
    result_value = COALESCE(sqlc.narg('result_value'), result_value),
    result_text = COALESCE(sqlc.narg('result_text'), result_text),
    result_unit_id = COALESCE(sqlc.narg('result_unit_id'), result_unit_id),
    pass_fail = COALESCE(sqlc.narg('pass_fail'), pass_fail),
    reviewed_by = COALESCE(sqlc.narg('reviewed_by'), reviewed_by),
    review_date = COALESCE(sqlc.narg('review_date'), review_date),
    review_notes = COALESCE(sqlc.narg('review_notes'), review_notes),
    notes = COALESCE(sqlc.narg('notes'), notes)
WHERE id = $1
RETURNING *;

-- name: DeleteLabTestAssignment :exec
DELETE FROM lab_test_assignments
WHERE id = $1;

-- ============================================================================
-- LAB TEST RESULTS
-- ============================================================================

-- name: CreateLabTestResult :one
INSERT INTO lab_test_results (
    test_assignment_id, test_date, analyst_id, equipment_id,
    parameter_name, result_value, result_text, result_unit_id,
    specification_min, specification_max, specification_target,
    is_in_spec, deviation,
    replicate_number, dilution_factor, preparation_details,
    test_temperature, test_humidity, test_conditions,
    system_suitability_pass, blank_value, reference_standard_value,
    raw_data_file, chromatogram_file, attachments, notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26
)
RETURNING *;

-- name: GetLabTestResultByID :one
SELECT 
    ltr.*,
    analyst.full_name as analyst_name,
    le.equipment_code,
    le.equipment_name
FROM lab_test_results ltr
LEFT JOIN users analyst ON ltr.analyst_id = analyst.id
LEFT JOIN lab_equipment le ON ltr.equipment_id = le.id
WHERE ltr.id = $1;

-- name: ListLabTestResults :many
SELECT 
    ltr.*,
    analyst.full_name as analyst_name,
    le.equipment_code
FROM lab_test_results ltr
LEFT JOIN users analyst ON ltr.analyst_id = analyst.id
LEFT JOIN lab_equipment le ON ltr.equipment_id = le.id
WHERE ltr.test_assignment_id = $1
ORDER BY ltr.replicate_number, ltr.created_at;

-- name: ListLabTestResultsByAnalyst :many
SELECT * FROM lab_test_results
WHERE analyst_id = $1
ORDER BY test_date DESC
LIMIT $2 OFFSET $3;

-- name: ListLabTestResultsOutOfSpec :many
SELECT 
    ltr.*,
    lta.sample_id,
    ls.sample_number,
    ls.batch_number,
    m.name as material_name
FROM lab_test_results ltr
LEFT JOIN lab_test_assignments lta ON ltr.test_assignment_id = lta.id
LEFT JOIN lab_samples ls ON lta.sample_id = ls.id
LEFT JOIN materials m ON ls.material_id = m.id
WHERE ltr.is_in_spec = FALSE
    AND ltr.test_date >= $1
    AND ltr.test_date <= $2
ORDER BY ltr.test_date DESC;

-- name: DeleteLabTestResult :exec
DELETE FROM lab_test_results
WHERE id = $1;

-- ============================================================================
-- ANALYST QUALIFICATIONS
-- ============================================================================

-- name: CreateAnalystQualification :one
INSERT INTO analyst_qualifications (
    analyst_id, test_method_id,
    qualification_date, qualified_by, expiry_date,
    training_completed, training_date, training_hours,
    assessment_score, assessment_notes,
    is_active, requalification_required, notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
RETURNING *;

-- name: GetAnalystQualificationByID :one
SELECT 
    aq.*,
    analyst.full_name as analyst_name,
    ltm.method_code,
    ltm.method_name,
    qualified.full_name as qualified_by_name
FROM analyst_qualifications aq
LEFT JOIN users analyst ON aq.analyst_id = analyst.id
LEFT JOIN lab_test_methods ltm ON aq.test_method_id = ltm.id
LEFT JOIN users qualified ON aq.qualified_by = qualified.id
WHERE aq.id = $1;

-- name: ListAnalystQualifications :many
SELECT 
    aq.*,
    analyst.full_name as analyst_name,
    ltm.method_code,
    ltm.method_name
FROM analyst_qualifications aq
LEFT JOIN users analyst ON aq.analyst_id = analyst.id
LEFT JOIN lab_test_methods ltm ON aq.test_method_id = ltm.id
WHERE aq.analyst_id = $1
ORDER BY aq.qualification_date DESC;

-- name: ListQualifiedAnalystsForMethod :many
SELECT 
    aq.*,
    u.full_name as analyst_name,
    u.email as analyst_email
FROM analyst_qualifications aq
LEFT JOIN users u ON aq.analyst_id = u.id
WHERE aq.test_method_id = $1
    AND aq.is_active = TRUE
    AND (aq.expiry_date IS NULL OR aq.expiry_date >= CURRENT_DATE)
ORDER BY u.full_name;

-- name: ListExpiringQualifications :many
SELECT 
    aq.*,
    u.full_name as analyst_name,
    ltm.method_code,
    ltm.method_name
FROM analyst_qualifications aq
LEFT JOIN users u ON aq.analyst_id = u.id
LEFT JOIN lab_test_methods ltm ON aq.test_method_id = ltm.id
WHERE aq.expiry_date <= $1
    AND aq.is_active = TRUE
ORDER BY aq.expiry_date;

-- name: CheckAnalystQualification :one
SELECT EXISTS(
    SELECT 1 FROM analyst_qualifications
    WHERE analyst_id = $1
        AND test_method_id = $2
        AND is_active = TRUE
        AND (expiry_date IS NULL OR expiry_date >= CURRENT_DATE)
) as is_qualified;

-- name: UpdateAnalystQualification :one
UPDATE analyst_qualifications
SET 
    qualification_date = COALESCE(sqlc.narg('qualification_date'), qualification_date),
    expiry_date = COALESCE(sqlc.narg('expiry_date'), expiry_date),
    is_active = COALESCE(sqlc.narg('is_active'), is_active),
    requalification_required = COALESCE(sqlc.narg('requalification_required'), requalification_required),
    notes = COALESCE(sqlc.narg('notes'), notes)
WHERE id = $1
RETURNING *;

-- name: DeleteAnalystQualification :exec
DELETE FROM analyst_qualifications
WHERE id = $1;

-- ============================================================================
-- CERTIFICATES OF ANALYSIS
-- ============================================================================

-- name: CreateCertificateOfAnalysis :one
INSERT INTO certificates_of_analysis (
    coa_number, material_id, batch_number, lot_number,
    quality_inspection_id, manufacture_date, expiry_date,
    quantity, unit_id, test_results,
    customer_id, sales_order_id, recipient_name, recipient_address,
    status, issue_date,
    prepared_by, prepared_date,
    reviewed_by, reviewed_date,
    approved_by, approved_date,
    digital_signature, signature_timestamp,
    pdf_file_path, template_used, notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27
)
RETURNING *;

-- name: GetCertificateOfAnalysisByID :one
SELECT 
    coa.*,
    m.name as material_name,
    m.sku as material_sku,
    cust.name as customer_name,
    prep.full_name as prepared_by_name,
    rev.full_name as reviewed_by_name,
    app.full_name as approved_by_name
FROM certificates_of_analysis coa
LEFT JOIN materials m ON coa.material_id = m.id
LEFT JOIN customers cust ON coa.customer_id = cust.id
LEFT JOIN users prep ON coa.prepared_by = prep.id
LEFT JOIN users rev ON coa.reviewed_by = rev.id
LEFT JOIN users app ON coa.approved_by = app.id
WHERE coa.id = $1;

-- name: GetCertificateOfAnalysisByNumber :one
SELECT * FROM certificates_of_analysis
WHERE coa_number = $1;

-- name: ListCertificatesOfAnalysis :many
SELECT 
    coa.id, coa.coa_number, coa.status,
    coa.material_id, m.name as material_name,
    coa.batch_number, coa.issue_date,
    cust.name as customer_name,
    coa.created_at
FROM certificates_of_analysis coa
LEFT JOIN materials m ON coa.material_id = m.id
LEFT JOIN customers cust ON coa.customer_id = cust.id
ORDER BY coa.created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListCertificatesOfAnalysisByMaterial :many
SELECT * FROM certificates_of_analysis
WHERE material_id = $1
ORDER BY created_at DESC;

-- name: ListCertificatesOfAnalysisByBatch :many
SELECT * FROM certificates_of_analysis
WHERE batch_number = $1
ORDER BY created_at DESC;

-- name: ListCertificatesOfAnalysisByCustomer :many
SELECT * FROM certificates_of_analysis
WHERE customer_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListCertificatesOfAnalysisByStatus :many
SELECT * FROM certificates_of_analysis
WHERE status = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: UpdateCertificateOfAnalysis :one
UPDATE certificates_of_analysis
SET 
    status = COALESCE(sqlc.narg('status'), status),
    issue_date = COALESCE(sqlc.narg('issue_date'), issue_date),
    test_results = COALESCE(sqlc.narg('test_results'), test_results),
    prepared_by = COALESCE(sqlc.narg('prepared_by'), prepared_by),
    prepared_date = COALESCE(sqlc.narg('prepared_date'), prepared_date),
    reviewed_by = COALESCE(sqlc.narg('reviewed_by'), reviewed_by),
    reviewed_date = COALESCE(sqlc.narg('reviewed_date'), reviewed_date),
    approved_by = COALESCE(sqlc.narg('approved_by'), approved_by),
    approved_date = COALESCE(sqlc.narg('approved_date'), approved_date),
    digital_signature = COALESCE(sqlc.narg('digital_signature'), digital_signature),
    signature_timestamp = COALESCE(sqlc.narg('signature_timestamp'), signature_timestamp),
    pdf_file_path = COALESCE(sqlc.narg('pdf_file_path'), pdf_file_path),
    notes = COALESCE(sqlc.narg('notes'), notes)
WHERE id = $1
RETURNING *;

-- name: DeleteCertificateOfAnalysis :exec
DELETE FROM certificates_of_analysis
WHERE id = $1;

-- ============================================================================
-- OOS INVESTIGATIONS
-- ============================================================================

-- name: CreateOOSInvestigation :one
INSERT INTO oos_investigations (
    oos_number, test_assignment_id, sample_id, ncr_id,
    oos_description, severity,
    initiated_by, initiated_date, investigator_id,
    status, notes, attachments
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
RETURNING *;

-- name: GetOOSInvestigationByID :one
SELECT 
    oos.*,
    ls.sample_number,
    ls.material_id,
    m.name as material_name,
    ls.batch_number,
    init.full_name as initiated_by_name,
    inv.full_name as investigator_name,
    rev.full_name as reviewed_by_name,
    app.full_name as approved_by_name
FROM oos_investigations oos
LEFT JOIN lab_samples ls ON oos.sample_id = ls.id
LEFT JOIN materials m ON ls.material_id = m.id
LEFT JOIN users init ON oos.initiated_by = init.id
LEFT JOIN users inv ON oos.investigator_id = inv.id
LEFT JOIN users rev ON oos.reviewed_by = rev.id
LEFT JOIN users app ON oos.approved_by = app.id
WHERE oos.id = $1;

-- name: GetOOSInvestigationByNumber :one
SELECT * FROM oos_investigations
WHERE oos_number = $1;

-- name: ListOOSInvestigations :many
SELECT 
    oos.id, oos.oos_number, oos.severity, oos.status,
    ls.sample_number, ls.batch_number,
    m.name as material_name,
    oos.initiated_date,
    inv.full_name as investigator_name,
    oos.created_at
FROM oos_investigations oos
LEFT JOIN lab_samples ls ON oos.sample_id = ls.id
LEFT JOIN materials m ON ls.material_id = m.id
LEFT JOIN users inv ON oos.investigator_id = inv.id
ORDER BY oos.created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListOOSInvestigationsByStatus :many
SELECT * FROM oos_investigations
WHERE status = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListOpenOOSInvestigations :many
SELECT * FROM oos_investigations
WHERE status = 'open'
ORDER BY severity DESC, initiated_date ASC;

-- name: UpdateOOSInvestigation :one
UPDATE oos_investigations
SET 
    status = COALESCE(sqlc.narg('status'), status),
    investigator_id = COALESCE(sqlc.narg('investigator_id'), investigator_id),
    phase_1_start_date = COALESCE(sqlc.narg('phase_1_start_date'), phase_1_start_date),
    phase_1_complete_date = COALESCE(sqlc.narg('phase_1_complete_date'), phase_1_complete_date),
    phase_1_findings = COALESCE(sqlc.narg('phase_1_findings'), phase_1_findings),
    lab_error_found = COALESCE(sqlc.narg('lab_error_found'), lab_error_found),
    lab_error_description = COALESCE(sqlc.narg('lab_error_description'), lab_error_description),
    phase_2_required = COALESCE(sqlc.narg('phase_2_required'), phase_2_required),
    phase_2_start_date = COALESCE(sqlc.narg('phase_2_start_date'), phase_2_start_date),
    phase_2_complete_date = COALESCE(sqlc.narg('phase_2_complete_date'), phase_2_complete_date),
    phase_2_findings = COALESCE(sqlc.narg('phase_2_findings'), phase_2_findings),
    root_cause = COALESCE(sqlc.narg('root_cause'), root_cause),
    final_conclusion = COALESCE(sqlc.narg('final_conclusion'), final_conclusion),
    corrective_action = COALESCE(sqlc.narg('corrective_action'), corrective_action),
    preventive_action = COALESCE(sqlc.narg('preventive_action'), preventive_action),
    batch_disposition = COALESCE(sqlc.narg('batch_disposition'), batch_disposition),
    reviewed_by = COALESCE(sqlc.narg('reviewed_by'), reviewed_by),
    approved_by = COALESCE(sqlc.narg('approved_by'), approved_by),
    closed_date = COALESCE(sqlc.narg('closed_date'), closed_date),
    notes = COALESCE(sqlc.narg('notes'), notes)
WHERE id = $1
RETURNING *;

-- name: DeleteOOSInvestigation :exec
DELETE FROM oos_investigations
WHERE id = $1;

-- ============================================================================
-- STABILITY STUDIES
-- ============================================================================

-- name: CreateStabilityStudy :one
INSERT INTO stability_studies (
    study_number, study_name, material_id, batch_number,
    study_type, storage_condition, study_duration_months,
    start_date, expected_end_date,
    test_schedule, test_methods,
    status, protocol_approved_by, protocol_approval_date,
    notes, attachments
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
)
RETURNING *;

-- name: GetStabilityStudyByID :one
SELECT 
    ss.*,
    m.name as material_name,
    m.sku as material_sku,
    prot_app.full_name as protocol_approved_by_name,
    rep_app.full_name as report_approved_by_name
FROM stability_studies ss
LEFT JOIN materials m ON ss.material_id = m.id
LEFT JOIN users prot_app ON ss.protocol_approved_by = prot_app.id
LEFT JOIN users rep_app ON ss.report_approved_by = rep_app.id
WHERE ss.id = $1;

-- name: GetStabilityStudyByNumber :one
SELECT * FROM stability_studies
WHERE study_number = $1;

-- name: ListStabilityStudies :many
SELECT 
    ss.id, ss.study_number, ss.study_name,
    ss.material_id, m.name as material_name,
    ss.batch_number, ss.study_type, ss.storage_condition,
    ss.status, ss.start_date, ss.expected_end_date,
    ss.created_at
FROM stability_studies ss
LEFT JOIN materials m ON ss.material_id = m.id
ORDER BY ss.created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListStabilityStudiesByMaterial :many
SELECT * FROM stability_studies
WHERE material_id = $1
ORDER BY start_date DESC;

-- name: ListStabilityStudiesByStatus :many
SELECT * FROM stability_studies
WHERE status = $1
ORDER BY start_date DESC
LIMIT $2 OFFSET $3;

-- name: ListActiveStabilityStudies :many
SELECT * FROM stability_studies
WHERE status = 'active'
ORDER BY start_date;

-- name: UpdateStabilityStudy :one
UPDATE stability_studies
SET 
    status = COALESCE(sqlc.narg('status'), status),
    actual_end_date = COALESCE(sqlc.narg('actual_end_date'), actual_end_date),
    results_summary = COALESCE(sqlc.narg('results_summary'), results_summary),
    conclusion = COALESCE(sqlc.narg('conclusion'), conclusion),
    shelf_life_recommendation = COALESCE(sqlc.narg('shelf_life_recommendation'), shelf_life_recommendation),
    report_approved_by = COALESCE(sqlc.narg('report_approved_by'), report_approved_by),
    report_approval_date = COALESCE(sqlc.narg('report_approval_date'), report_approval_date),
    notes = COALESCE(sqlc.narg('notes'), notes)
WHERE id = $1
RETURNING *;

-- name: DeleteStabilityStudy :exec
DELETE FROM stability_studies
WHERE id = $1;

-- ============================================================================
-- STABILITY SAMPLES
-- ============================================================================

-- name: CreateStabilitySample :one
INSERT INTO stability_samples (
    stability_study_id, lab_sample_id,
    time_point_months, scheduled_pull_date, actual_pull_date,
    testing_due_date, notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
RETURNING *;

-- name: GetStabilitySampleByID :one
SELECT 
    stab_s.*,
    ls.sample_number,
    ss.study_number,
    ss.study_name
FROM stability_samples stab_s
LEFT JOIN lab_samples ls ON stab_s.lab_sample_id = ls.id
LEFT JOIN stability_studies ss ON stab_s.stability_study_id = ss.id
WHERE stab_s.id = $1;

-- name: ListStabilitySamplesByStudy :many
SELECT 
    stab_s.*,
    ls.sample_number
FROM stability_samples stab_s
LEFT JOIN lab_samples ls ON stab_s.lab_sample_id = ls.id
WHERE stab_s.stability_study_id = $1
ORDER BY stab_s.time_point_months;

-- name: ListStabilitySamplesDue :many
SELECT 
    stab_s.*,
    ss.study_number,
    ss.study_name,
    ls.sample_number
FROM stability_samples stab_s
LEFT JOIN stability_studies ss ON stab_s.stability_study_id = ss.id
LEFT JOIN lab_samples ls ON stab_s.lab_sample_id = ls.id
WHERE stab_s.scheduled_pull_date <= $1
    AND stab_s.actual_pull_date IS NULL
ORDER BY stab_s.scheduled_pull_date;

-- name: UpdateStabilitySample :one
UPDATE stability_samples
SET 
    actual_pull_date = COALESCE(sqlc.narg('actual_pull_date'), actual_pull_date),
    testing_completed = COALESCE(sqlc.narg('testing_completed'), testing_completed),
    testing_completed_date = COALESCE(sqlc.narg('testing_completed_date'), testing_completed_date),
    results_summary = COALESCE(sqlc.narg('results_summary'), results_summary),
    all_tests_passed = COALESCE(sqlc.narg('all_tests_passed'), all_tests_passed),
    notes = COALESCE(sqlc.narg('notes'), notes)
WHERE id = $1
RETURNING *;

-- name: DeleteStabilitySample :exec
DELETE FROM stability_samples
WHERE id = $1;

-- ============================================================================
-- STATISTICS & REPORTS
-- ============================================================================

-- name: GetLabDashboardStats :one
SELECT 
    (SELECT COUNT(*) FROM lab_samples WHERE sample_status = 'pending') as pending_samples,
    (SELECT COUNT(*) FROM lab_test_assignments WHERE status IN ('pending', 'in_progress')) as pending_tests,
    (SELECT COUNT(*) FROM lab_test_assignments WHERE due_date < CURRENT_DATE AND status IN ('pending', 'in_progress')) as overdue_tests,
    (SELECT COUNT(*) FROM lab_equipment WHERE calibration_status = 'overdue') as overdue_calibrations,
    (SELECT COUNT(*) FROM oos_investigations WHERE status = 'open') as open_oos;

-- name: GetAnalystProductivity :many
SELECT 
    u.id,
    u.full_name,
    COUNT(DISTINCT lta.id) as total_assignments,
    COUNT(DISTINCT CASE WHEN lta.status = 'pass' THEN lta.id END) as completed_tests,
    COUNT(DISTINCT CASE WHEN lta.status = 'fail' THEN lta.id END) as failed_tests,
    AVG(EXTRACT(EPOCH FROM (lta.completed_date - lta.started_date))/3600) as avg_hours_per_test
FROM users u
LEFT JOIN lab_test_assignments lta ON u.id = lta.assigned_to
WHERE lta.completed_date >= $1 AND lta.completed_date <= $2
GROUP BY u.id, u.full_name
ORDER BY completed_tests DESC;
