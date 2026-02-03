-- Quality Management System Migration
-- Comprehensive quality control, inspections, NCR, and supplier quality tracking

-- ============================================================================
-- ENUMS & TYPES
-- ============================================================================

-- Quality inspection types
CREATE TYPE quality_inspection_type AS ENUM (
    'incoming',      -- Incoming materials from suppliers
    'in_process',    -- During production/manufacturing
    'final',         -- Final product before shipping
    'periodic',      -- Regular scheduled checks
    'audit',         -- Internal quality audits
    'customer_return' -- Returned goods inspection
);

-- Quality inspection status
CREATE TYPE quality_inspection_status AS ENUM (
    'pending',       -- Awaiting inspection
    'in_progress',   -- Currently being inspected
    'passed',        -- Inspection passed
    'failed',        -- Inspection failed
    'partial',       -- Partially passed (some items OK, some not)
    'on_hold',       -- Awaiting decision
    'cancelled'      -- Inspection cancelled
);

-- Quality status for materials/batches
CREATE TYPE quality_status AS ENUM (
    'unrestricted',  -- Approved for use/sale
    'quarantine',    -- Hold for inspection
    'blocked',       -- Not usable
    'rejected'       -- Rejected, must be returned or scrapped
);

-- NCR (Non-Conformance Report) status
CREATE TYPE ncr_status AS ENUM (
    'open',          -- Newly reported
    'investigating', -- Root cause analysis in progress
    'action_required', -- Corrective action needed
    'in_progress',   -- Action being taken
    'resolved',      -- Issue resolved
    'closed',        -- Verified and closed
    'cancelled'      -- NCR cancelled
);

-- NCR severity
CREATE TYPE ncr_severity AS ENUM (
    'critical',      -- Production/shipment stop
    'major',         -- Significant impact
    'minor',         -- Low impact
    'observation'    -- Improvement opportunity
);

-- NCR type
CREATE TYPE ncr_type AS ENUM (
    'supplier',      -- Supplier quality issue
    'process',       -- Internal process issue
    'customer',      -- Customer complaint
    'audit',         -- Found during audit
    'other'
);

-- ============================================================================
-- INSPECTION CRITERIA TEMPLATES
-- ============================================================================

-- Define what needs to be checked during inspections
CREATE TABLE IF NOT EXISTS quality_inspection_criteria (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    criteria_type VARCHAR(50) NOT NULL, -- 'visual', 'measurement', 'functional', 'document'
    specification TEXT,                  -- Expected value or range
    unit_id INT REFERENCES measure_units(id) ON DELETE SET NULL,
    tolerance_min DECIMAL(15, 4),       -- Minimum acceptable value
    tolerance_max DECIMAL(15, 4),       -- Maximum acceptable value
    is_critical BOOLEAN DEFAULT FALSE,  -- Critical to quality
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_quality_criteria_type ON quality_inspection_criteria(criteria_type);
CREATE INDEX idx_quality_criteria_active ON quality_inspection_criteria(is_active);

CREATE TRIGGER trg_update_quality_inspection_criteria_updated_at
BEFORE UPDATE ON quality_inspection_criteria
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

-- ============================================================================
-- QUALITY INSPECTIONS
-- ============================================================================

-- Main inspection records
CREATE TABLE IF NOT EXISTS quality_inspections (
    id SERIAL PRIMARY KEY,
    inspection_number VARCHAR(50) UNIQUE NOT NULL, -- QI-2026-0001
    inspection_type quality_inspection_type NOT NULL,
    inspection_status quality_inspection_status DEFAULT 'pending',
    
    -- What is being inspected
    material_id INT REFERENCES materials(id) ON DELETE CASCADE,
    batch_number VARCHAR(100),
    lot_number VARCHAR(100),
    quantity DECIMAL(15, 4) NOT NULL,
    unit_id INT REFERENCES measure_units(id) ON DELETE SET NULL,
    
    -- Related documents
    purchase_order_id INT REFERENCES purchase_orders(id) ON DELETE SET NULL,
    sales_order_id INT REFERENCES sales_orders(id) ON DELETE SET NULL,
    stock_movement_id INT REFERENCES stock_movements(id) ON DELETE SET NULL,
    
    -- Supplier info (for incoming inspections)
    supplier_id INT REFERENCES suppliers(id) ON DELETE SET NULL,
    
    -- Inspection details
    inspection_date TIMESTAMP WITH TIME ZONE,
    inspector_id INT REFERENCES users(id) ON DELETE SET NULL,
    approved_by_id INT REFERENCES users(id) ON DELETE SET NULL,
    
    -- Results summary
    quantity_passed DECIMAL(15, 4),
    quantity_failed DECIMAL(15, 4),
    quantity_on_hold DECIMAL(15, 4),
    
    -- Decision
    final_decision quality_status,
    decision_date TIMESTAMP WITH TIME ZONE,
    
    -- Notes
    notes TEXT,
    attachments JSONB, -- Store file paths/URLs
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_quality_inspections_number ON quality_inspections(inspection_number);
CREATE INDEX idx_quality_inspections_type ON quality_inspections(inspection_type);
CREATE INDEX idx_quality_inspections_status ON quality_inspections(inspection_status);
CREATE INDEX idx_quality_inspections_material ON quality_inspections(material_id);
CREATE INDEX idx_quality_inspections_po ON quality_inspections(purchase_order_id);
CREATE INDEX idx_quality_inspections_supplier ON quality_inspections(supplier_id);
CREATE INDEX idx_quality_inspections_date ON quality_inspections(inspection_date);
CREATE INDEX idx_quality_inspections_batch ON quality_inspections(batch_number);

CREATE TRIGGER trg_update_quality_inspections_updated_at
BEFORE UPDATE ON quality_inspections
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

-- ============================================================================
-- INSPECTION RESULTS (DETAILS)
-- ============================================================================

-- Detailed measurements and observations for each inspection
CREATE TABLE IF NOT EXISTS quality_inspection_results (
    id SERIAL PRIMARY KEY,
    inspection_id INT NOT NULL REFERENCES quality_inspections(id) ON DELETE CASCADE,
    criteria_id INT REFERENCES quality_inspection_criteria(id) ON DELETE SET NULL,
    
    -- Measurement details
    criteria_name VARCHAR(255) NOT NULL, -- Denormalized for history
    measured_value DECIMAL(15, 4),
    text_value TEXT,                     -- For non-numeric observations
    
    -- Result
    is_passed BOOLEAN,
    deviation DECIMAL(15, 4),            -- How far from spec
    
    -- Context
    sample_number INT,                   -- Which sample in the batch
    notes TEXT,
    photo_urls JSONB,                    -- Store photo references
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_quality_results_inspection ON quality_inspection_results(inspection_id);
CREATE INDEX idx_quality_results_criteria ON quality_inspection_results(criteria_id);
CREATE INDEX idx_quality_results_passed ON quality_inspection_results(is_passed);

-- ============================================================================
-- NON-CONFORMANCE REPORTS (NCR)
-- ============================================================================

CREATE TABLE IF NOT EXISTS non_conformance_reports (
    id SERIAL PRIMARY KEY,
    ncr_number VARCHAR(50) UNIQUE NOT NULL, -- NCR-2026-0001
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    
    -- Classification
    ncr_type ncr_type NOT NULL,
    severity ncr_severity NOT NULL,
    status ncr_status DEFAULT 'open',
    
    -- What's affected
    material_id INT REFERENCES materials(id) ON DELETE SET NULL,
    batch_number VARCHAR(100),
    quantity_affected DECIMAL(15, 4),
    unit_id INT REFERENCES measure_units(id) ON DELETE SET NULL,
    
    -- Related records
    inspection_id INT REFERENCES quality_inspections(id) ON DELETE SET NULL,
    supplier_id INT REFERENCES suppliers(id) ON DELETE SET NULL,
    customer_id INT REFERENCES customers(id) ON DELETE SET NULL,
    purchase_order_id INT REFERENCES purchase_orders(id) ON DELETE SET NULL,
    sales_order_id INT REFERENCES sales_orders(id) ON DELETE SET NULL,
    
    -- Root cause analysis
    root_cause TEXT,
    root_cause_analysis_by INT REFERENCES users(id) ON DELETE SET NULL,
    root_cause_date TIMESTAMP WITH TIME ZONE,
    
    -- Corrective action
    corrective_action TEXT,
    preventive_action TEXT,
    action_assigned_to INT REFERENCES users(id) ON DELETE SET NULL,
    action_due_date DATE,
    action_completed_date DATE,
    
    -- Disposition
    disposition VARCHAR(100), -- 'rework', 'scrap', 'return_to_supplier', 'use_as_is', 'sort'
    cost_impact DECIMAL(15, 2),
    
    -- Workflow
    reported_by INT REFERENCES users(id) ON DELETE SET NULL,
    reported_date TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    closed_by INT REFERENCES users(id) ON DELETE SET NULL,
    closed_date TIMESTAMP WITH TIME ZONE,
    
    -- Documentation
    attachments JSONB,
    notes TEXT,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_ncr_number ON non_conformance_reports(ncr_number);
CREATE INDEX idx_ncr_status ON non_conformance_reports(status);
CREATE INDEX idx_ncr_severity ON non_conformance_reports(severity);
CREATE INDEX idx_ncr_type ON non_conformance_reports(ncr_type);
CREATE INDEX idx_ncr_material ON non_conformance_reports(material_id);
CREATE INDEX idx_ncr_supplier ON non_conformance_reports(supplier_id);
CREATE INDEX idx_ncr_reported_date ON non_conformance_reports(reported_date);

CREATE TRIGGER trg_update_ncr_updated_at
BEFORE UPDATE ON non_conformance_reports
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

-- ============================================================================
-- QUALITY HOLDS
-- ============================================================================

-- Track materials that are quarantined or blocked
CREATE TABLE IF NOT EXISTS quality_holds (
    id SERIAL PRIMARY KEY,
    hold_number VARCHAR(50) UNIQUE NOT NULL, -- QH-2026-0001
    
    -- What's on hold
    material_id INT NOT NULL REFERENCES materials(id) ON DELETE CASCADE,
    warehouse_id INT REFERENCES warehouses(id) ON DELETE SET NULL,
    batch_number VARCHAR(100),
    lot_number VARCHAR(100),
    quantity DECIMAL(15, 4) NOT NULL,
    unit_id INT REFERENCES measure_units(id) ON DELETE SET NULL,
    
    -- Hold details
    quality_status quality_status NOT NULL,
    hold_reason TEXT NOT NULL,
    
    -- Related records
    inspection_id INT REFERENCES quality_inspections(id) ON DELETE SET NULL,
    ncr_id INT REFERENCES non_conformance_reports(id) ON DELETE SET NULL,
    
    -- Workflow
    placed_by INT REFERENCES users(id) ON DELETE SET NULL,
    placed_date TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    expected_release_date DATE,
    
    -- Release
    is_released BOOLEAN DEFAULT FALSE,
    released_by INT REFERENCES users(id) ON DELETE SET NULL,
    released_date TIMESTAMP WITH TIME ZONE,
    release_notes TEXT,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_quality_holds_number ON quality_holds(hold_number);
CREATE INDEX idx_quality_holds_material ON quality_holds(material_id);
CREATE INDEX idx_quality_holds_warehouse ON quality_holds(warehouse_id);
CREATE INDEX idx_quality_holds_status ON quality_holds(quality_status);
CREATE INDEX idx_quality_holds_released ON quality_holds(is_released);
CREATE INDEX idx_quality_holds_batch ON quality_holds(batch_number);

CREATE TRIGGER trg_update_quality_holds_updated_at
BEFORE UPDATE ON quality_holds
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

-- ============================================================================
-- SUPPLIER QUALITY RATINGS
-- ============================================================================

-- Track supplier quality performance over time
CREATE TABLE IF NOT EXISTS supplier_quality_ratings (
    id SERIAL PRIMARY KEY,
    supplier_id INT NOT NULL REFERENCES suppliers(id) ON DELETE CASCADE,
    
    -- Rating period
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    
    -- Metrics
    total_inspections INT DEFAULT 0,
    passed_inspections INT DEFAULT 0,
    failed_inspections INT DEFAULT 0,
    total_quantity_received DECIMAL(15, 4) DEFAULT 0,
    quantity_rejected DECIMAL(15, 4) DEFAULT 0,
    
    -- Defect tracking
    total_defects INT DEFAULT 0,
    critical_defects INT DEFAULT 0,
    major_defects INT DEFAULT 0,
    minor_defects INT DEFAULT 0,
    
    -- NCRs
    ncr_count INT DEFAULT 0,
    
    -- Calculated scores (0-100)
    quality_score DECIMAL(5, 2),         -- Overall quality score
    defect_rate DECIMAL(5, 2),           -- Percentage
    rejection_rate DECIMAL(5, 2),        -- Percentage
    
    -- Rating
    rating VARCHAR(20),                  -- 'excellent', 'good', 'fair', 'poor'
    
    -- Notes
    notes TEXT,
    calculated_by INT REFERENCES users(id) ON DELETE SET NULL,
    calculation_date TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(supplier_id, period_start, period_end)
);

CREATE INDEX idx_supplier_quality_supplier ON supplier_quality_ratings(supplier_id);
CREATE INDEX idx_supplier_quality_period ON supplier_quality_ratings(period_start, period_end);
CREATE INDEX idx_supplier_quality_rating ON supplier_quality_ratings(rating);

CREATE TRIGGER trg_update_supplier_quality_ratings_updated_at
BEFORE UPDATE ON supplier_quality_ratings
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

-- ============================================================================
-- MATERIAL QUALITY SPECIFICATIONS
-- ============================================================================

-- Link materials to required inspection criteria
CREATE TABLE IF NOT EXISTS material_quality_specs (
    id SERIAL PRIMARY KEY,
    material_id INT NOT NULL REFERENCES materials(id) ON DELETE CASCADE,
    criteria_id INT NOT NULL REFERENCES quality_inspection_criteria(id) ON DELETE CASCADE,
    
    -- Override defaults if needed
    is_required BOOLEAN DEFAULT TRUE,
    custom_tolerance_min DECIMAL(15, 4),
    custom_tolerance_max DECIMAL(15, 4),
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(material_id, criteria_id)
);

CREATE INDEX idx_material_quality_specs_material ON material_quality_specs(material_id);
CREATE INDEX idx_material_quality_specs_criteria ON material_quality_specs(criteria_id);

CREATE TRIGGER trg_update_material_quality_specs_updated_at
BEFORE UPDATE ON material_quality_specs
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

-- ============================================================================
-- FUNCTIONS & TRIGGERS
-- ============================================================================

-- Function to generate inspection number
CREATE OR REPLACE FUNCTION generate_inspection_number()
RETURNS TEXT AS $$
DECLARE
    next_num INT;
    year_part TEXT;
BEGIN
    year_part := TO_CHAR(CURRENT_DATE, 'YYYY');
    SELECT COALESCE(MAX(CAST(SUBSTRING(inspection_number FROM 9) AS INT)), 0) + 1
    INTO next_num
    FROM quality_inspections
    WHERE inspection_number LIKE 'QI-' || year_part || '-%';
    
    RETURN 'QI-' || year_part || '-' || LPAD(next_num::TEXT, 4, '0');
END;
$$ LANGUAGE plpgsql;

-- Function to generate NCR number
CREATE OR REPLACE FUNCTION generate_ncr_number()
RETURNS TEXT AS $$
DECLARE
    next_num INT;
    year_part TEXT;
BEGIN
    year_part := TO_CHAR(CURRENT_DATE, 'YYYY');
    SELECT COALESCE(MAX(CAST(SUBSTRING(ncr_number FROM 10) AS INT)), 0) + 1
    INTO next_num
    FROM non_conformance_reports
    WHERE ncr_number LIKE 'NCR-' || year_part || '-%';
    
    RETURN 'NCR-' || year_part || '-' || LPAD(next_num::TEXT, 4, '0');
END;
$$ LANGUAGE plpgsql;

-- Function to generate hold number
CREATE OR REPLACE FUNCTION generate_hold_number()
RETURNS TEXT AS $$
DECLARE
    next_num INT;
    year_part TEXT;
BEGIN
    year_part := TO_CHAR(CURRENT_DATE, 'YYYY');
    SELECT COALESCE(MAX(CAST(SUBSTRING(hold_number FROM 9) AS INT)), 0) + 1
    INTO next_num
    FROM quality_holds
    WHERE hold_number LIKE 'QH-' || year_part || '-%';
    
    RETURN 'QH-' || year_part || '-' || LPAD(next_num::TEXT, 4, '0');
END;
$$ LANGUAGE plpgsql;

-- Auto-generate inspection number
CREATE OR REPLACE FUNCTION set_inspection_number()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.inspection_number IS NULL OR NEW.inspection_number = '' THEN
        NEW.inspection_number := generate_inspection_number();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_set_inspection_number
BEFORE INSERT ON quality_inspections
FOR EACH ROW
EXECUTE FUNCTION set_inspection_number();

-- Auto-generate NCR number
CREATE OR REPLACE FUNCTION set_ncr_number()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.ncr_number IS NULL OR NEW.ncr_number = '' THEN
        NEW.ncr_number := generate_ncr_number();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_set_ncr_number
BEFORE INSERT ON non_conformance_reports
FOR EACH ROW
EXECUTE FUNCTION set_ncr_number();

-- Auto-generate hold number
CREATE OR REPLACE FUNCTION set_hold_number()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.hold_number IS NULL OR NEW.hold_number = '' THEN
        NEW.hold_number := generate_hold_number();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_set_hold_number
BEFORE INSERT ON quality_holds
FOR EACH ROW
EXECUTE FUNCTION set_hold_number();


-- ============================================================================
-- VIEWS FOR REPORTING
-- ============================================================================

-- Supplier quality summary view
CREATE OR REPLACE VIEW v_supplier_quality_summary AS
SELECT 
    s.id AS supplier_id,
    s.name AS supplier_name,
    COUNT(DISTINCT qi.id) AS total_inspections,
    COUNT(DISTINCT CASE WHEN qi.inspection_status = 'passed' THEN qi.id END) AS passed_inspections,
    COUNT(DISTINCT CASE WHEN qi.inspection_status = 'failed' THEN qi.id END) AS failed_inspections,
    COUNT(DISTINCT ncr.id) AS total_ncrs,
    COUNT(DISTINCT CASE WHEN ncr.severity = 'critical' THEN ncr.id END) AS critical_ncrs,
    ROUND(
        CASE 
            WHEN COUNT(DISTINCT qi.id) > 0 
            THEN (COUNT(DISTINCT CASE WHEN qi.inspection_status = 'passed' THEN qi.id END)::DECIMAL / COUNT(DISTINCT qi.id) * 100)
            ELSE 0 
        END, 
    2) AS pass_rate,
    MAX(sqr.quality_score) AS latest_quality_score,
    MAX(sqr.rating) AS latest_rating
FROM suppliers s
LEFT JOIN quality_inspections qi ON s.id = qi.supplier_id
LEFT JOIN non_conformance_reports ncr ON s.id = ncr.supplier_id
LEFT JOIN supplier_quality_ratings sqr ON s.id = sqr.supplier_id
GROUP BY s.id, s.name;

-- Material quality performance view
CREATE OR REPLACE VIEW v_material_quality_summary AS
SELECT 
    m.id AS material_id,
    m.name AS material_name,
    m.sku,
    COUNT(DISTINCT qi.id) AS total_inspections,
    COUNT(DISTINCT CASE WHEN qi.inspection_status = 'passed' THEN qi.id END) AS passed_inspections,
    COUNT(DISTINCT CASE WHEN qi.inspection_status = 'failed' THEN qi.id END) AS failed_inspections,
    COUNT(DISTINCT ncr.id) AS total_ncrs,
    COUNT(DISTINCT qh.id) AS total_holds,
    COUNT(DISTINCT CASE WHEN qh.is_released = FALSE THEN qh.id END) AS active_holds,
    ROUND(
        CASE 
            WHEN COUNT(DISTINCT qi.id) > 0 
            THEN (COUNT(DISTINCT CASE WHEN qi.inspection_status = 'passed' THEN qi.id END)::DECIMAL / COUNT(DISTINCT qi.id) * 100)
            ELSE 0 
        END, 
    2) AS pass_rate
FROM materials m
LEFT JOIN quality_inspections qi ON m.id = qi.material_id
LEFT JOIN non_conformance_reports ncr ON m.id = ncr.material_id
LEFT JOIN quality_holds qh ON m.id = qh.material_id
GROUP BY m.id, m.name, m.sku;

-- Active quality holds view
CREATE OR REPLACE VIEW v_active_quality_holds AS
SELECT 
    qh.hold_number,
    qh.quality_status,
    m.name AS material_name,
    m.sku,
    qh.batch_number,
    qh.quantity,
    mu.abbreviation AS unit,
    w.name AS warehouse_name,
    qh.hold_reason,
    qh.placed_date,
    qh.expected_release_date,
    u.full_name AS placed_by_name,
    CASE 
        WHEN qh.expected_release_date < CURRENT_DATE THEN TRUE 
        ELSE FALSE 
    END AS is_overdue
FROM quality_holds qh
JOIN materials m ON qh.material_id = m.id
LEFT JOIN measure_units mu ON qh.unit_id = mu.id
LEFT JOIN warehouses w ON qh.warehouse_id = w.id
LEFT JOIN users u ON qh.placed_by = u.id
WHERE qh.is_released = FALSE;

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON TABLE quality_inspections IS 'Main quality inspection records for incoming, in-process, and final inspections';
COMMENT ON TABLE quality_inspection_results IS 'Detailed measurements and observations for each inspection criterion';
COMMENT ON TABLE non_conformance_reports IS 'Track quality issues, defects, and corrective actions (NCR/CAR)';
COMMENT ON TABLE quality_holds IS 'Materials placed on quality hold/quarantine';
COMMENT ON TABLE supplier_quality_ratings IS 'Periodic supplier quality performance metrics';
COMMENT ON TABLE quality_inspection_criteria IS 'Templates defining what to inspect and acceptable tolerances';
COMMENT ON TABLE material_quality_specs IS 'Link materials to their required quality criteria';