-- ============================================================================
-- QUALITY INSPECTION CRITERIA
-- ============================================================================

-- name: CreateQualityInspectionCriteria :one
INSERT INTO quality_inspection_criteria (
    name, description, criteria_type, specification,
    unit_id, tolerance_min, tolerance_max, is_critical, is_active
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING *;

-- name: GetQualityInspectionCriteriaByID :one
SELECT * FROM quality_inspection_criteria
WHERE id = $1;

-- name: ListQualityInspectionCriteria :many
SELECT * FROM quality_inspection_criteria
WHERE is_active = TRUE
ORDER BY name;

-- name: ListAllQualityInspectionCriteria :many
SELECT * FROM quality_inspection_criteria
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateQualityInspectionCriteria :one
UPDATE quality_inspection_criteria
SET 
    name = COALESCE(sqlc.narg('name'), name),
    description = COALESCE(sqlc.narg('description'), description),
    criteria_type = COALESCE(sqlc.narg('criteria_type'), criteria_type),
    specification = COALESCE(sqlc.narg('specification'), specification),
    unit_id = COALESCE(sqlc.narg('unit_id'), unit_id),
    tolerance_min = COALESCE(sqlc.narg('tolerance_min'), tolerance_min),
    tolerance_max = COALESCE(sqlc.narg('tolerance_max'), tolerance_max),
    is_critical = COALESCE(sqlc.narg('is_critical'), is_critical),
    is_active = COALESCE(sqlc.narg('is_active'), is_active)
WHERE id = $1
RETURNING *;

-- name: DeleteQualityInspectionCriteria :exec
DELETE FROM quality_inspection_criteria
WHERE id = $1;

-- name: SearchQualityInspectionCriteria :many
SELECT * FROM quality_inspection_criteria
WHERE name ILIKE '%' || $1 || '%'
    OR description ILIKE '%' || $1 || '%'
ORDER BY name
LIMIT $2 OFFSET $3;

-- ============================================================================
-- QUALITY INSPECTIONS
-- ============================================================================

-- name: CreateQualityInspection :one
INSERT INTO quality_inspections (
    inspection_number, inspection_type, inspection_status,
    material_id, batch_number, lot_number, quantity, unit_id,
    purchase_order_id, sales_order_id, stock_movement_id, supplier_id,
    inspection_date, inspector_id, notes, attachments
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
)
RETURNING *;

-- name: GetQualityInspectionByID :one
SELECT 
    qi.*,
    m.name as material_name,
    m.sku as material_sku,
    u.abbreviation as unit_abbreviation,
    sup.name as supplier_name,
    insp.full_name as inspector_name,
    app.full_name as approved_by_name
FROM quality_inspections qi
LEFT JOIN materials m ON qi.material_id = m.id
LEFT JOIN measure_units u ON qi.unit_id = u.id
LEFT JOIN suppliers sup ON qi.supplier_id = sup.id
LEFT JOIN users insp ON qi.inspector_id = insp.id
LEFT JOIN users app ON qi.approved_by_id = app.id
WHERE qi.id = $1;

-- name: GetQualityInspectionByNumber :one
SELECT * FROM quality_inspections
WHERE inspection_number = $1;

-- name: ListQualityInspections :many
SELECT 
    qi.id, qi.inspection_number, qi.inspection_type, qi.inspection_status,
    qi.material_id, m.name as material_name, m.sku as material_sku,
    qi.batch_number, qi.quantity, u.abbreviation as unit_abbreviation,
    qi.inspection_date, qi.final_decision,
    sup.name as supplier_name,
    insp.full_name as inspector_name,
    qi.created_at
FROM quality_inspections qi
LEFT JOIN materials m ON qi.material_id = m.id
LEFT JOIN measure_units u ON qi.unit_id = u.id
LEFT JOIN suppliers sup ON qi.supplier_id = sup.id
LEFT JOIN users insp ON qi.inspector_id = insp.id
ORDER BY qi.created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListQualityInspectionsByStatus :many
SELECT * FROM quality_inspections
WHERE inspection_status = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListQualityInspectionsByType :many
SELECT * FROM quality_inspections
WHERE inspection_type = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListQualityInspectionsByMaterial :many
SELECT * FROM quality_inspections
WHERE material_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListQualityInspectionsBySupplier :many
SELECT * FROM quality_inspections
WHERE supplier_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListQualityInspectionsByBatch :many
SELECT * FROM quality_inspections
WHERE batch_number = $1
ORDER BY created_at DESC;

-- name: ListPendingInspections :many
SELECT * FROM quality_inspections
WHERE inspection_status IN ('pending', 'in_progress')
ORDER BY inspection_date ASC, created_at ASC
LIMIT $1 OFFSET $2;

-- name: UpdateQualityInspection :one
UPDATE quality_inspections
SET 
    inspection_status = COALESCE(sqlc.narg('inspection_status'), inspection_status),
    inspection_date = COALESCE(sqlc.narg('inspection_date'), inspection_date),
    inspector_id = COALESCE(sqlc.narg('inspector_id'), inspector_id),
    approved_by_id = COALESCE(sqlc.narg('approved_by_id'), approved_by_id),
    quantity_passed = COALESCE(sqlc.narg('quantity_passed'), quantity_passed),
    quantity_failed = COALESCE(sqlc.narg('quantity_failed'), quantity_failed),
    quantity_on_hold = COALESCE(sqlc.narg('quantity_on_hold'), quantity_on_hold),
    final_decision = COALESCE(sqlc.narg('final_decision'), final_decision),
    decision_date = COALESCE(sqlc.narg('decision_date'), decision_date),
    notes = COALESCE(sqlc.narg('notes'), notes),
    attachments = COALESCE(sqlc.narg('attachments'), attachments)
WHERE id = $1
RETURNING *;

-- name: DeleteQualityInspection :exec
DELETE FROM quality_inspections
WHERE id = $1;

-- name: CountQualityInspectionsByStatus :one
SELECT COUNT(*) FROM quality_inspections
WHERE inspection_status = $1;

-- name: GetInspectionStatsByMaterial :one
SELECT 
    COUNT(*) as total_inspections,
    COUNT(CASE WHEN inspection_status = 'passed' THEN 1 END) as passed_count,
    COUNT(CASE WHEN inspection_status = 'failed' THEN 1 END) as failed_count,
    ROUND(
        CASE 
            WHEN COUNT(*) > 0 
            THEN (COUNT(CASE WHEN inspection_status = 'passed' THEN 1 END)::DECIMAL / COUNT(*) * 100)
            ELSE 0 
        END, 
    2) as pass_rate
FROM quality_inspections
WHERE material_id = $1;

-- ============================================================================
-- QUALITY INSPECTION RESULTS
-- ============================================================================

-- name: CreateQualityInspectionResult :one
INSERT INTO quality_inspection_results (
    inspection_id, criteria_id, criteria_name,
    measured_value, text_value, is_passed,
    deviation, sample_number, notes, photo_urls
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING *;

-- name: GetQualityInspectionResultByID :one
SELECT * FROM quality_inspection_results
WHERE id = $1;

-- name: ListQualityInspectionResults :many
SELECT 
    qir.*,
    qic.name as criteria_name,
    qic.criteria_type,
    qic.tolerance_min,
    qic.tolerance_max,
    qic.is_critical
FROM quality_inspection_results qir
LEFT JOIN quality_inspection_criteria qic ON qir.criteria_id = qic.id
WHERE qir.inspection_id = $1
ORDER BY qir.created_at;

-- name: ListFailedInspectionResults :many
SELECT * FROM quality_inspection_results
WHERE inspection_id = $1 AND is_passed = FALSE
ORDER BY created_at;

-- name: DeleteQualityInspectionResult :exec
DELETE FROM quality_inspection_results
WHERE id = $1;

-- name: BulkCreateQualityInspectionResults :copyfrom
INSERT INTO quality_inspection_results (
    inspection_id, criteria_id, criteria_name,
    measured_value, text_value, is_passed,
    deviation, sample_number, notes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
);

-- ============================================================================
-- NON-CONFORMANCE REPORTS (NCR)
-- ============================================================================

-- name: CreateNonConformanceReport :one
INSERT INTO non_conformance_reports (
    ncr_number, title, description, ncr_type, severity, status,
    material_id, batch_number, quantity_affected, unit_id,
    inspection_id, supplier_id, customer_id,
    purchase_order_id, sales_order_id,
    reported_by, reported_date, notes, attachments
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
)
RETURNING *;

-- name: GetNonConformanceReportByID :one
SELECT 
    ncr.*,
    m.name as material_name,
    m.sku as material_sku,
    sup.name as supplier_name,
    cust.name as customer_name,
    rep.full_name as reported_by_name,
    assigned.full_name as action_assigned_to_name
FROM non_conformance_reports ncr
LEFT JOIN materials m ON ncr.material_id = m.id
LEFT JOIN suppliers sup ON ncr.supplier_id = sup.id
LEFT JOIN customers cust ON ncr.customer_id = cust.id
LEFT JOIN users rep ON ncr.reported_by = rep.id
LEFT JOIN users assigned ON ncr.action_assigned_to = assigned.id
WHERE ncr.id = $1;

-- name: GetNonConformanceReportByNumber :one
SELECT * FROM non_conformance_reports
WHERE ncr_number = $1;

-- name: ListNonConformanceReports :many
SELECT 
    ncr.id, ncr.ncr_number, ncr.title, ncr.ncr_type, ncr.severity, ncr.status,
    ncr.material_id, m.name as material_name,
    ncr.batch_number, ncr.quantity_affected,
    ncr.reported_date, ncr.action_due_date,
    rep.full_name as reported_by_name,
    assigned.full_name as action_assigned_to_name
FROM non_conformance_reports ncr
LEFT JOIN materials m ON ncr.material_id = m.id
LEFT JOIN users rep ON ncr.reported_by = rep.id
LEFT JOIN users assigned ON ncr.action_assigned_to = assigned.id
ORDER BY ncr.reported_date DESC
LIMIT $1 OFFSET $2;

-- name: ListNonConformanceReportsByStatus :many
SELECT * FROM non_conformance_reports
WHERE status = $1
ORDER BY reported_date DESC
LIMIT $2 OFFSET $3;

-- name: ListNonConformanceReportsBySeverity :many
SELECT * FROM non_conformance_reports
WHERE severity = $1
ORDER BY reported_date DESC
LIMIT $2 OFFSET $3;

-- name: ListNonConformanceReportsByType :many
SELECT * FROM non_conformance_reports
WHERE ncr_type = $1
ORDER BY reported_date DESC
LIMIT $2 OFFSET $3;

-- name: ListNonConformanceReportsByMaterial :many
SELECT * FROM non_conformance_reports
WHERE material_id = $1
ORDER BY reported_date DESC;

-- name: ListNonConformanceReportsBySupplier :many
SELECT * FROM non_conformance_reports
WHERE supplier_id = $1
ORDER BY reported_date DESC;

-- name: ListOpenNonConformanceReports :many
SELECT * FROM non_conformance_reports
WHERE status IN ('open', 'investigating', 'action_required', 'in_progress')
ORDER BY severity DESC, reported_date ASC
LIMIT $1 OFFSET $2;

-- name: ListOverdueNCRActions :many
SELECT * FROM non_conformance_reports
WHERE status IN ('open', 'investigating', 'action_required', 'in_progress')
    AND action_due_date < CURRENT_DATE
ORDER BY action_due_date ASC;

-- name: UpdateNonConformanceReport :one
UPDATE non_conformance_reports
SET 
    status = COALESCE(sqlc.narg('status'), status),
    root_cause = COALESCE(sqlc.narg('root_cause'), root_cause),
    root_cause_analysis_by = COALESCE(sqlc.narg('root_cause_analysis_by'), root_cause_analysis_by),
    root_cause_date = COALESCE(sqlc.narg('root_cause_date'), root_cause_date),
    corrective_action = COALESCE(sqlc.narg('corrective_action'), corrective_action),
    preventive_action = COALESCE(sqlc.narg('preventive_action'), preventive_action),
    action_assigned_to = COALESCE(sqlc.narg('action_assigned_to'), action_assigned_to),
    action_due_date = COALESCE(sqlc.narg('action_due_date'), action_due_date),
    action_completed_date = COALESCE(sqlc.narg('action_completed_date'), action_completed_date),
    disposition = COALESCE(sqlc.narg('disposition'), disposition),
    cost_impact = COALESCE(sqlc.narg('cost_impact'), cost_impact),
    closed_by = COALESCE(sqlc.narg('closed_by'), closed_by),
    closed_date = COALESCE(sqlc.narg('closed_date'), closed_date),
    notes = COALESCE(sqlc.narg('notes'), notes),
    attachments = COALESCE(sqlc.narg('attachments'), attachments)
WHERE id = $1
RETURNING *;

-- name: DeleteNonConformanceReport :exec
DELETE FROM non_conformance_reports
WHERE id = $1;

-- name: CountNonConformanceReportsByStatus :one
SELECT COUNT(*) FROM non_conformance_reports
WHERE status = $1;

-- ============================================================================
-- QUALITY HOLDS
-- ============================================================================

-- name: CreateQualityHold :one
INSERT INTO quality_holds (
    hold_number, material_id, warehouse_id, batch_number, lot_number,
    quantity, unit_id, quality_status, hold_reason,
    inspection_id, ncr_id, placed_by, placed_date, expected_release_date
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
)
RETURNING *;

-- name: GetQualityHoldByID :one
SELECT 
    qh.*,
    m.name as material_name,
    m.sku as material_sku,
    w.name as warehouse_name,
    u.abbreviation as unit_abbreviation,
    placed.full_name as placed_by_name,
    released.full_name as released_by_name
FROM quality_holds qh
LEFT JOIN materials m ON qh.material_id = m.id
LEFT JOIN warehouses w ON qh.warehouse_id = w.id
LEFT JOIN measure_units u ON qh.unit_id = u.id
LEFT JOIN users placed ON qh.placed_by = placed.id
LEFT JOIN users released ON qh.released_by = released.id
WHERE qh.id = $1;

-- name: GetQualityHoldByNumber :one
SELECT * FROM quality_holds
WHERE hold_number = $1;

-- name: ListQualityHolds :many
SELECT 
    qh.id, qh.hold_number, qh.quality_status,
    qh.material_id, m.name as material_name, m.sku as material_sku,
    qh.batch_number, qh.quantity, u.abbreviation as unit_abbreviation,
    w.name as warehouse_name,
    qh.hold_reason, qh.placed_date, qh.expected_release_date,
    qh.is_released, qh.released_date,
    placed.full_name as placed_by_name
FROM quality_holds qh
LEFT JOIN materials m ON qh.material_id = m.id
LEFT JOIN warehouses w ON qh.warehouse_id = w.id
LEFT JOIN measure_units u ON qh.unit_id = u.id
LEFT JOIN users placed ON qh.placed_by = placed.id
ORDER BY qh.placed_date DESC
LIMIT $1 OFFSET $2;

-- name: ListActiveQualityHolds :many
SELECT * FROM quality_holds
WHERE is_released = FALSE
ORDER BY placed_date DESC
LIMIT $1 OFFSET $2;

-- name: ListQualityHoldsByMaterial :many
SELECT * FROM quality_holds
WHERE material_id = $1
ORDER BY placed_date DESC;

-- name: ListQualityHoldsByBatch :many
SELECT * FROM quality_holds
WHERE batch_number = $1
ORDER BY placed_date DESC;

-- name: ListQualityHoldsByStatus :many
SELECT * FROM quality_holds
WHERE quality_status = $1 AND is_released = FALSE
ORDER BY placed_date DESC
LIMIT $2 OFFSET $3;

-- name: UpdateQualityHold :one
UPDATE quality_holds
SET 
    quality_status = COALESCE(sqlc.narg('quality_status'), quality_status),
    hold_reason = COALESCE(sqlc.narg('hold_reason'), hold_reason),
    expected_release_date = COALESCE(sqlc.narg('expected_release_date'), expected_release_date),
    is_released = COALESCE(sqlc.narg('is_released'), is_released),
    released_by = COALESCE(sqlc.narg('released_by'), released_by),
    released_date = COALESCE(sqlc.narg('released_date'), released_date),
    release_notes = COALESCE(sqlc.narg('release_notes'), release_notes)
WHERE id = $1
RETURNING *;

-- name: ReleaseQualityHold :one
UPDATE quality_holds
SET 
    is_released = TRUE,
    released_by = $2,
    released_date = CURRENT_TIMESTAMP,
    release_notes = $3
WHERE id = $1
RETURNING *;

-- name: DeleteQualityHold :exec
DELETE FROM quality_holds
WHERE id = $1;

-- ============================================================================
-- SUPPLIER QUALITY RATINGS
-- ============================================================================

-- name: CreateSupplierQualityRating :one
INSERT INTO supplier_quality_ratings (
    supplier_id, period_start, period_end,
    total_inspections, passed_inspections, failed_inspections,
    total_quantity_received, quantity_rejected,
    total_defects, critical_defects, major_defects, minor_defects,
    ncr_count, quality_score, defect_rate, rejection_rate,
    rating, notes, calculated_by, calculation_date
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20
)
RETURNING *;

-- name: GetSupplierQualityRatingByID :one
SELECT 
    sqr.*,
    s.name as supplier_name,
    u.full_name as calculated_by_name
FROM supplier_quality_ratings sqr
LEFT JOIN suppliers s ON sqr.supplier_id = s.id
LEFT JOIN users u ON sqr.calculated_by = u.id
WHERE sqr.id = $1;

-- name: ListSupplierQualityRatings :many
SELECT 
    sqr.*,
    s.name as supplier_name
FROM supplier_quality_ratings sqr
LEFT JOIN suppliers s ON sqr.supplier_id = s.id
ORDER BY sqr.period_end DESC, sqr.quality_score DESC
LIMIT $1 OFFSET $2;

-- name: ListSupplierQualityRatingsBySupplier :many
SELECT * FROM supplier_quality_ratings
WHERE supplier_id = $1
ORDER BY period_end DESC;

-- name: GetLatestSupplierQualityRating :one
SELECT * FROM supplier_quality_ratings
WHERE supplier_id = $1
ORDER BY period_end DESC
LIMIT 1;

-- name: ListSuppliersByQualityRating :many
SELECT 
    s.id,
    s.name,
    sqr.quality_score,
    sqr.rating,
    sqr.defect_rate,
    sqr.rejection_rate,
    sqr.period_end
FROM suppliers s
LEFT JOIN LATERAL (
    SELECT * FROM supplier_quality_ratings
    WHERE supplier_id = s.id
    ORDER BY period_end DESC
    LIMIT 1
) sqr ON TRUE
ORDER BY sqr.quality_score DESC NULLS LAST;

-- name: UpdateSupplierQualityRating :one
UPDATE supplier_quality_ratings
SET 
    total_inspections = COALESCE(sqlc.narg('total_inspections'), total_inspections),
    passed_inspections = COALESCE(sqlc.narg('passed_inspections'), passed_inspections),
    failed_inspections = COALESCE(sqlc.narg('failed_inspections'), failed_inspections),
    total_quantity_received = COALESCE(sqlc.narg('total_quantity_received'), total_quantity_received),
    quantity_rejected = COALESCE(sqlc.narg('quantity_rejected'), quantity_rejected),
    total_defects = COALESCE(sqlc.narg('total_defects'), total_defects),
    critical_defects = COALESCE(sqlc.narg('critical_defects'), critical_defects),
    major_defects = COALESCE(sqlc.narg('major_defects'), major_defects),
    minor_defects = COALESCE(sqlc.narg('minor_defects'), minor_defects),
    ncr_count = COALESCE(sqlc.narg('ncr_count'), ncr_count),
    quality_score = COALESCE(sqlc.narg('quality_score'), quality_score),
    defect_rate = COALESCE(sqlc.narg('defect_rate'), defect_rate),
    rejection_rate = COALESCE(sqlc.narg('rejection_rate'), rejection_rate),
    rating = COALESCE(sqlc.narg('rating'), rating),
    notes = COALESCE(sqlc.narg('notes'), notes)
WHERE id = $1
RETURNING *;

-- name: DeleteSupplierQualityRating :exec
DELETE FROM supplier_quality_ratings
WHERE id = $1;

-- ============================================================================
-- MATERIAL QUALITY SPECS
-- ============================================================================

-- name: CreateMaterialQualitySpec :one
INSERT INTO material_quality_specs (
    material_id, criteria_id, is_required,
    custom_tolerance_min, custom_tolerance_max
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING *;

-- name: GetMaterialQualitySpecByID :one
SELECT * FROM material_quality_specs
WHERE id = $1;

-- name: ListMaterialQualitySpecs :many
SELECT 
    mqs.*,
    m.name as material_name,
    qic.name as criteria_name,
    qic.criteria_type,
    qic.is_critical
FROM material_quality_specs mqs
LEFT JOIN materials m ON mqs.material_id = m.id
LEFT JOIN quality_inspection_criteria qic ON mqs.criteria_id = qic.id
WHERE mqs.material_id = $1;

-- name: UpdateMaterialQualitySpec :one
UPDATE material_quality_specs
SET 
    is_required = COALESCE(sqlc.narg('is_required'), is_required),
    custom_tolerance_min = COALESCE(sqlc.narg('custom_tolerance_min'), custom_tolerance_min),
    custom_tolerance_max = COALESCE(sqlc.narg('custom_tolerance_max'), custom_tolerance_max)
WHERE id = $1
RETURNING *;

-- name: DeleteMaterialQualitySpec :exec
DELETE FROM material_quality_specs
WHERE id = $1;

-- name: DeleteMaterialQualitySpecsByMaterial :exec
DELETE FROM material_quality_specs
WHERE material_id = $1;

-- ============================================================================
-- STATISTICS & REPORTS
-- ============================================================================

-- name: GetQualityDashboardStats :one
SELECT 
    (SELECT COUNT(*) FROM quality_inspections WHERE inspection_status = 'pending') as pending_inspections,
    (SELECT COUNT(*) FROM quality_inspections WHERE inspection_status = 'in_progress') as in_progress_inspections,
    (SELECT COUNT(*) FROM quality_holds WHERE is_released = FALSE) as active_holds,
    (SELECT COUNT(*) FROM non_conformance_reports WHERE status IN ('open', 'investigating', 'action_required', 'in_progress')) as open_ncrs,
    (SELECT COUNT(*) FROM non_conformance_reports WHERE status IN ('open', 'investigating', 'action_required', 'in_progress') AND action_due_date < CURRENT_DATE) as overdue_ncrs;

-- name: GetQualityInspectionTrends :many
SELECT 
    DATE_TRUNC('month', inspection_date) as month,
    COUNT(*) as total_inspections,
    COUNT(CASE WHEN inspection_status = 'passed' THEN 1 END) as passed,
    COUNT(CASE WHEN inspection_status = 'failed' THEN 1 END) as failed,
    ROUND(
        CASE 
            WHEN COUNT(*) > 0 
            THEN (COUNT(CASE WHEN inspection_status = 'passed' THEN 1 END)::DECIMAL / COUNT(*) * 100)
            ELSE 0 
        END, 
    2) as pass_rate
FROM quality_inspections
WHERE inspection_date >= $1 AND inspection_date <= $2
GROUP BY DATE_TRUNC('month', inspection_date)
ORDER BY month;

-- name: GetTopDefectiveMaterials :many
SELECT 
    m.id,
    m.name,
    m.sku,
    COUNT(DISTINCT qi.id) as total_inspections,
    COUNT(DISTINCT CASE WHEN qi.inspection_status = 'failed' THEN qi.id END) as failed_inspections,
    COUNT(DISTINCT ncr.id) as ncr_count,
    ROUND(
        CASE 
            WHEN COUNT(DISTINCT qi.id) > 0 
            THEN (COUNT(DISTINCT CASE WHEN qi.inspection_status = 'failed' THEN qi.id END)::DECIMAL / COUNT(DISTINCT qi.id) * 100)
            ELSE 0 
        END, 
    2) as failure_rate
FROM materials m
LEFT JOIN quality_inspections qi ON m.id = qi.material_id
LEFT JOIN non_conformance_reports ncr ON m.id = ncr.material_id
GROUP BY m.id, m.name, m.sku
HAVING COUNT(DISTINCT qi.id) > 0
ORDER BY failure_rate DESC, ncr_count DESC
LIMIT $1;
