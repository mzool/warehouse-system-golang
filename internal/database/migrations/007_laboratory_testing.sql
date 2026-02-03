-- Laboratory Testing System Migration
-- Comprehensive lab sample management, test methods, equipment, and compliance

-- ============================================================================
-- ENUMS & TYPES
-- ============================================================================

-- Lab sample status
CREATE TYPE lab_sample_status AS ENUM (
    'pending',          -- Sample collected, awaiting testing
    'in_testing',       -- Currently being tested
    'completed',        -- Testing complete
    'on_hold',          -- Testing paused
    'cancelled',        -- Sample cancelled
    'consumed',         -- Sample fully used
    'retained',         -- Kept for retention period
    'disposed'          -- Properly disposed
);

-- Sample type
CREATE TYPE lab_sample_type AS ENUM (
    'raw_material',     -- Incoming materials
    'finished_product', -- Final products
    'in_process',       -- WIP/intermediate
    'environmental',    -- Environmental monitoring
    'stability',        -- Stability study samples
    'retain',           -- Retention samples
    'reference',        -- Reference standards
    'control'           -- Quality control samples
);

-- Test method status
CREATE TYPE test_method_status AS ENUM (
    'draft',            -- Being developed
    'under_review',     -- In review process
    'active',           -- Approved and in use
    'inactive',         -- No longer used
    'superseded'        -- Replaced by newer version
);

-- Test result status
CREATE TYPE test_result_status AS ENUM (
    'pending',          -- Not yet tested
    'in_progress',      -- Testing underway
    'pass',             -- Met specifications
    'fail',             -- Did not meet specs
    'out_of_spec',      -- OOS - requires investigation
    'retest',           -- Needs retesting
    'cancelled',        -- Test cancelled
    'invalidated'       -- Results invalidated
);

-- Equipment calibration status
CREATE TYPE calibration_status AS ENUM (
    'calibrated',       -- Currently calibrated
    'due_soon',         -- Due within warning period
    'overdue',          -- Past due date
    'out_of_service',   -- Not available
    'under_calibration' -- Being calibrated
);

-- Certificate of Analysis status
CREATE TYPE coa_status AS ENUM (
    'draft',
    'pending_review',
    'approved',
    'issued',
    'void'
);

-- ============================================================================
-- TEST METHODS CATALOG
-- ============================================================================

-- Standard test methods and procedures
CREATE TABLE IF NOT EXISTS lab_test_methods (
    id SERIAL PRIMARY KEY,
    method_code VARCHAR(100) UNIQUE NOT NULL,  -- TM-001, ASTM-D1234
    method_name VARCHAR(255) NOT NULL,
    description TEXT,
    
    -- Standard reference
    standard_reference VARCHAR(255),            -- 'ASTM D1234-20', 'USP <791>'
    standard_organization VARCHAR(100),         -- 'ASTM', 'ISO', 'USP', 'Internal'
    
    -- Method details
    test_type VARCHAR(100) NOT NULL,           -- 'chemical', 'physical', 'microbiological'
    test_category VARCHAR(100),                -- 'identity', 'purity', 'strength', 'appearance'
    methodology TEXT,                          -- Detailed procedure
    
    -- Requirements
    sample_size DECIMAL(15, 4),
    sample_unit_id INT REFERENCES measure_units(id) ON DELETE SET NULL,
    preparation_time INT,                      -- Minutes
    test_duration INT,                         -- Minutes
    
    -- Equipment needed
    required_equipment JSONB,                  -- Array of equipment IDs/names
    
    -- Acceptance criteria
    specification_limits JSONB,                -- {min, max, target, type: 'range'|'limit'|'text'}
    
    -- Version control
    version VARCHAR(20) DEFAULT '1.0',
    effective_date DATE,
    supersedes_method_id INT REFERENCES lab_test_methods(id) ON DELETE SET NULL,
    
    -- Status & approval
    status test_method_status DEFAULT 'draft',
    approved_by INT REFERENCES users(id) ON DELETE SET NULL,
    approval_date DATE,
    
    -- Documentation
    attachments JSONB,                         -- SOPs, worksheets
    notes TEXT,
    
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_lab_test_methods_code ON lab_test_methods(method_code);
CREATE INDEX idx_lab_test_methods_status ON lab_test_methods(status);
CREATE INDEX idx_lab_test_methods_type ON lab_test_methods(test_type);
CREATE INDEX idx_lab_test_methods_active ON lab_test_methods(is_active);

CREATE TRIGGER trg_update_lab_test_methods_updated_at
BEFORE UPDATE ON lab_test_methods
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

-- ============================================================================
-- LAB EQUIPMENT & INSTRUMENTS
-- ============================================================================

CREATE TABLE IF NOT EXISTS lab_equipment (
    id SERIAL PRIMARY KEY,
    equipment_code VARCHAR(100) UNIQUE NOT NULL,  -- EQ-001
    equipment_name VARCHAR(255) NOT NULL,
    equipment_type VARCHAR(100) NOT NULL,          -- 'balance', 'HPLC', 'spectrophotometer'
    manufacturer VARCHAR(255),
    model_number VARCHAR(100),
    serial_number VARCHAR(100),
    
    -- Location
    location VARCHAR(255),                         -- Lab room/bench
    warehouse_id INT REFERENCES warehouses(id) ON DELETE SET NULL,
    
    -- Calibration
    calibration_frequency_days INT,                -- How often to calibrate
    last_calibration_date DATE,
    next_calibration_date DATE,
    calibration_status calibration_status DEFAULT 'calibrated',
    calibration_certificate VARCHAR(255),
    
    -- Maintenance
    last_maintenance_date DATE,
    next_maintenance_date DATE,
    maintenance_notes TEXT,
    
    -- Status
    is_operational BOOLEAN DEFAULT TRUE,
    is_qualified BOOLEAN DEFAULT TRUE,             -- IQ/OQ/PQ status
    qualification_date DATE,
    
    -- Documentation
    attachments JSONB,                             -- Manuals, certs, validation docs
    notes TEXT,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_lab_equipment_code ON lab_equipment(equipment_code);
CREATE INDEX idx_lab_equipment_type ON lab_equipment(equipment_type);
CREATE INDEX idx_lab_equipment_calibration_status ON lab_equipment(calibration_status);
CREATE INDEX idx_lab_equipment_next_calibration ON lab_equipment(next_calibration_date);

CREATE TRIGGER trg_update_lab_equipment_updated_at
BEFORE UPDATE ON lab_equipment
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

-- ============================================================================
-- LAB SAMPLES
-- ============================================================================

CREATE TABLE IF NOT EXISTS lab_samples (
    id SERIAL PRIMARY KEY,
    sample_number VARCHAR(100) UNIQUE NOT NULL,    -- LS-2026-0001
    sample_type lab_sample_type NOT NULL,
    sample_status lab_sample_status DEFAULT 'pending',
    
    -- What is being sampled
    material_id INT REFERENCES materials(id) ON DELETE CASCADE,
    batch_number VARCHAR(100),
    lot_number VARCHAR(100),
    
    -- Related records
    quality_inspection_id INT REFERENCES quality_inspections(id) ON DELETE SET NULL,
    purchase_order_id INT REFERENCES purchase_orders(id) ON DELETE SET NULL,
    stock_transaction_id INT REFERENCES stock_movements(id) ON DELETE SET NULL,
    
    -- Sample details
    sample_quantity DECIMAL(15, 4) NOT NULL,
    sample_unit_id INT REFERENCES measure_units(id) ON DELETE SET NULL,
    container_type VARCHAR(100),                   -- 'bottle', 'vial', 'bag'
    container_count INT DEFAULT 1,
    storage_location VARCHAR(255),
    storage_conditions VARCHAR(255),               -- '2-8°C', 'room temp', 'freezer'
    
    -- Collection details
    collected_by INT REFERENCES users(id) ON DELETE SET NULL,
    collection_date TIMESTAMP WITH TIME ZONE,
    collection_method TEXT,
    sampling_plan VARCHAR(255),                    -- Reference to sampling SOP
    
    -- Chain of custody
    received_by_lab INT REFERENCES users(id) ON DELETE SET NULL,
    lab_received_date TIMESTAMP WITH TIME ZONE,
    transferred_to INT REFERENCES users(id) ON DELETE SET NULL,
    transfer_date TIMESTAMP WITH TIME ZONE,
    chain_of_custody JSONB,                        -- Track all transfers
    
    -- Retention
    retention_required BOOLEAN DEFAULT FALSE,
    retention_period_days INT,
    retention_expiry_date DATE,
    disposed_date DATE,
    disposed_by INT REFERENCES users(id) ON DELETE SET NULL,
    disposal_method TEXT,
    
    -- External lab
    is_external_lab BOOLEAN DEFAULT FALSE,
    external_lab_name VARCHAR(255),
    external_lab_reference VARCHAR(100),
    sent_to_lab_date DATE,
    expected_results_date DATE,
    
    -- Documentation
    attachments JSONB,
    notes TEXT,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_lab_samples_number ON lab_samples(sample_number);
CREATE INDEX idx_lab_samples_type ON lab_samples(sample_type);
CREATE INDEX idx_lab_samples_status ON lab_samples(sample_status);
CREATE INDEX idx_lab_samples_material ON lab_samples(material_id);
CREATE INDEX idx_lab_samples_batch ON lab_samples(batch_number);
CREATE INDEX idx_lab_samples_inspection ON lab_samples(quality_inspection_id);
CREATE INDEX idx_lab_samples_collection_date ON lab_samples(collection_date);

CREATE TRIGGER trg_update_lab_samples_updated_at
BEFORE UPDATE ON lab_samples
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

-- ============================================================================
-- LAB TEST ASSIGNMENTS
-- ============================================================================

-- Which tests need to be performed on which samples
CREATE TABLE IF NOT EXISTS lab_test_assignments (
    id SERIAL PRIMARY KEY,
    sample_id INT NOT NULL REFERENCES lab_samples(id) ON DELETE CASCADE,
    test_method_id INT NOT NULL REFERENCES lab_test_methods(id) ON DELETE RESTRICT,
    
    -- Priority & scheduling
    priority INT DEFAULT 5,                        -- 1=highest, 10=lowest
    requested_date DATE,
    scheduled_date DATE,
    due_date DATE,
    
    -- Assignment
    assigned_to INT REFERENCES users(id) ON DELETE SET NULL,  -- Analyst
    assigned_date TIMESTAMP WITH TIME ZONE,
    
    -- Execution
    started_date TIMESTAMP WITH TIME ZONE,
    completed_date TIMESTAMP WITH TIME ZONE,
    
    -- Status
    status test_result_status DEFAULT 'pending',
    is_rush BOOLEAN DEFAULT FALSE,
    
    -- Results summary
    result_value DECIMAL(15, 6),
    result_text TEXT,
    result_unit_id INT REFERENCES measure_units(id) ON DELETE SET NULL,
    pass_fail BOOLEAN,
    
    -- Review
    reviewed_by INT REFERENCES users(id) ON DELETE SET NULL,
    review_date TIMESTAMP WITH TIME ZONE,
    review_notes TEXT,
    
    -- Retest tracking
    is_retest BOOLEAN DEFAULT FALSE,
    original_test_id INT REFERENCES lab_test_assignments(id) ON DELETE SET NULL,
    retest_reason TEXT,
    
    notes TEXT,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_lab_test_assignments_sample ON lab_test_assignments(sample_id);
CREATE INDEX idx_lab_test_assignments_method ON lab_test_assignments(test_method_id);
CREATE INDEX idx_lab_test_assignments_status ON lab_test_assignments(status);
CREATE INDEX idx_lab_test_assignments_assigned ON lab_test_assignments(assigned_to);
CREATE INDEX idx_lab_test_assignments_due ON lab_test_assignments(due_date);

CREATE TRIGGER trg_update_lab_test_assignments_updated_at
BEFORE UPDATE ON lab_test_assignments
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

-- ============================================================================
-- LAB TEST RESULTS (DETAILED)
-- ============================================================================

-- Detailed individual measurements and observations
CREATE TABLE IF NOT EXISTS lab_test_results (
    id SERIAL PRIMARY KEY,
    test_assignment_id INT NOT NULL REFERENCES lab_test_assignments(id) ON DELETE CASCADE,
    
    -- Test execution details
    test_date TIMESTAMP WITH TIME ZONE NOT NULL,
    analyst_id INT REFERENCES users(id) ON DELETE SET NULL,
    equipment_id INT REFERENCES lab_equipment(id) ON DELETE SET NULL,
    
    -- Results
    parameter_name VARCHAR(255) NOT NULL,          -- 'pH', 'Moisture', 'Assay'
    result_value DECIMAL(15, 6),
    result_text TEXT,                              -- For non-numeric results
    result_unit_id INT REFERENCES measure_units(id) ON DELETE SET NULL,
    
    -- Specifications
    specification_min DECIMAL(15, 6),
    specification_max DECIMAL(15, 6),
    specification_target DECIMAL(15, 6),
    
    -- Evaluation
    is_in_spec BOOLEAN,
    deviation DECIMAL(15, 6),
    
    -- Measurement details
    replicate_number INT,                          -- Which replicate (1, 2, 3)
    dilution_factor DECIMAL(10, 4),
    preparation_details TEXT,
    
    -- Environmental conditions
    test_temperature DECIMAL(5, 2),
    test_humidity DECIMAL(5, 2),
    test_conditions JSONB,
    
    -- Quality checks
    system_suitability_pass BOOLEAN,
    blank_value DECIMAL(15, 6),
    reference_standard_value DECIMAL(15, 6),
    
    -- Documentation
    raw_data_file VARCHAR(255),
    chromatogram_file VARCHAR(255),
    attachments JSONB,
    notes TEXT,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_lab_test_results_assignment ON lab_test_results(test_assignment_id);
CREATE INDEX idx_lab_test_results_analyst ON lab_test_results(analyst_id);
CREATE INDEX idx_lab_test_results_equipment ON lab_test_results(equipment_id);
CREATE INDEX idx_lab_test_results_date ON lab_test_results(test_date);

-- ============================================================================
-- ANALYST QUALIFICATIONS
-- ============================================================================

-- Track which analysts are qualified for which test methods
CREATE TABLE IF NOT EXISTS analyst_qualifications (
    id SERIAL PRIMARY KEY,
    analyst_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    test_method_id INT NOT NULL REFERENCES lab_test_methods(id) ON DELETE CASCADE,
    
    -- Qualification details
    qualification_date DATE NOT NULL,
    qualified_by INT REFERENCES users(id) ON DELETE SET NULL,
    expiry_date DATE,
    
    -- Training
    training_completed BOOLEAN DEFAULT TRUE,
    training_date DATE,
    training_hours DECIMAL(5, 2),
    
    -- Assessment
    assessment_score DECIMAL(5, 2),                -- Percentage
    assessment_notes TEXT,
    
    -- Status
    is_active BOOLEAN DEFAULT TRUE,
    requalification_required BOOLEAN DEFAULT FALSE,
    
    notes TEXT,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(analyst_id, test_method_id)
);

CREATE INDEX idx_analyst_qualifications_analyst ON analyst_qualifications(analyst_id);
CREATE INDEX idx_analyst_qualifications_method ON analyst_qualifications(test_method_id);
CREATE INDEX idx_analyst_qualifications_expiry ON analyst_qualifications(expiry_date);

CREATE TRIGGER trg_update_analyst_qualifications_updated_at
BEFORE UPDATE ON analyst_qualifications
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

-- ============================================================================
-- CERTIFICATES OF ANALYSIS (COA)
-- ============================================================================

CREATE TABLE IF NOT EXISTS certificates_of_analysis (
    id SERIAL PRIMARY KEY,
    coa_number VARCHAR(100) UNIQUE NOT NULL,       -- COA-2026-0001
    
    -- What's being certified
    material_id INT NOT NULL REFERENCES materials(id) ON DELETE RESTRICT,
    batch_number VARCHAR(100) NOT NULL,
    lot_number VARCHAR(100),
    
    -- Related records
    quality_inspection_id INT REFERENCES quality_inspections(id) ON DELETE SET NULL,
    
    -- Manufacturing details
    manufacture_date DATE,
    expiry_date DATE,
    quantity DECIMAL(15, 4),
    unit_id INT REFERENCES measure_units(id) ON DELETE SET NULL,
    
    -- Test results summary (from lab_test_assignments)
    test_results JSONB,                            -- Summary of all test results
    
    -- Customer/recipient
    customer_id INT REFERENCES customers(id) ON DELETE SET NULL,
    sales_order_id INT REFERENCES sales_orders(id) ON DELETE SET NULL,
    recipient_name VARCHAR(255),
    recipient_address TEXT,
    
    -- CoA details
    status coa_status DEFAULT 'draft',
    issue_date DATE,
    
    -- Approval workflow
    prepared_by INT REFERENCES users(id) ON DELETE SET NULL,
    prepared_date DATE,
    reviewed_by INT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_date DATE,
    approved_by INT REFERENCES users(id) ON DELETE SET NULL,
    approved_date DATE,
    
    -- Digital signature
    digital_signature TEXT,                        -- Cryptographic signature
    signature_timestamp TIMESTAMP WITH TIME ZONE,
    
    -- Documentation
    pdf_file_path VARCHAR(500),                    -- Generated PDF
    template_used VARCHAR(100),
    
    notes TEXT,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_coa_number ON certificates_of_analysis(coa_number);
CREATE INDEX idx_coa_material ON certificates_of_analysis(material_id);
CREATE INDEX idx_coa_batch ON certificates_of_analysis(batch_number);
CREATE INDEX idx_coa_status ON certificates_of_analysis(status);
CREATE INDEX idx_coa_customer ON certificates_of_analysis(customer_id);
CREATE INDEX idx_coa_issue_date ON certificates_of_analysis(issue_date);

CREATE TRIGGER trg_update_coa_updated_at
BEFORE UPDATE ON certificates_of_analysis
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

-- ============================================================================
-- OUT-OF-SPECIFICATION (OOS) INVESTIGATIONS
-- ============================================================================

-- Track investigations for out-of-spec results
CREATE TABLE IF NOT EXISTS oos_investigations (
    id SERIAL PRIMARY KEY,
    oos_number VARCHAR(100) UNIQUE NOT NULL,       -- OOS-2026-0001
    
    -- Related records
    test_assignment_id INT NOT NULL REFERENCES lab_test_assignments(id) ON DELETE RESTRICT,
    sample_id INT REFERENCES lab_samples(id) ON DELETE SET NULL,
    ncr_id INT REFERENCES non_conformance_reports(id) ON DELETE SET NULL,
    
    -- Investigation details
    oos_description TEXT NOT NULL,
    severity ncr_severity DEFAULT 'major',
    
    -- Phase I Investigation (Lab Error)
    phase_1_start_date DATE,
    phase_1_complete_date DATE,
    phase_1_findings TEXT,
    lab_error_found BOOLEAN,
    lab_error_description TEXT,
    
    -- Phase II Investigation (Process/Material)
    phase_2_required BOOLEAN DEFAULT FALSE,
    phase_2_start_date DATE,
    phase_2_complete_date DATE,
    phase_2_findings TEXT,
    root_cause TEXT,
    
    -- Retest
    retest_required BOOLEAN DEFAULT FALSE,
    retest_completed BOOLEAN DEFAULT FALSE,
    retest_results TEXT,
    
    -- Conclusion
    final_conclusion TEXT,
    corrective_action TEXT,
    preventive_action TEXT,
    
    -- Impact assessment
    batch_disposition VARCHAR(100),                -- 'reject', 'rework', 'accept', 'investigate_further'
    impact_on_other_batches TEXT,
    
    -- Workflow
    initiated_by INT REFERENCES users(id) ON DELETE SET NULL,
    initiated_date TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    investigator_id INT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_by INT REFERENCES users(id) ON DELETE SET NULL,
    approved_by INT REFERENCES users(id) ON DELETE SET NULL,
    
    status VARCHAR(50) DEFAULT 'open',
    closed_date DATE,
    
    notes TEXT,
    attachments JSONB,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_oos_investigations_number ON oos_investigations(oos_number);
CREATE INDEX idx_oos_investigations_test ON oos_investigations(test_assignment_id);
CREATE INDEX idx_oos_investigations_status ON oos_investigations(status);

CREATE TRIGGER trg_update_oos_investigations_updated_at
BEFORE UPDATE ON oos_investigations
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

-- ============================================================================
-- STABILITY TESTING
-- ============================================================================

-- Track stability studies for products
CREATE TABLE IF NOT EXISTS stability_studies (
    id SERIAL PRIMARY KEY,
    study_number VARCHAR(100) UNIQUE NOT NULL,     -- STB-2026-0001
    study_name VARCHAR(255) NOT NULL,
    
    -- Product
    material_id INT NOT NULL REFERENCES materials(id) ON DELETE RESTRICT,
    batch_number VARCHAR(100) NOT NULL,
    
    -- Study design
    study_type VARCHAR(100),                       -- 'long_term', 'accelerated', 'intermediate'
    storage_condition VARCHAR(255) NOT NULL,       -- '25°C/60% RH', '40°C/75% RH'
    study_duration_months INT,
    
    -- Schedule
    start_date DATE NOT NULL,
    expected_end_date DATE,
    actual_end_date DATE,
    
    -- Testing schedule (time points in months)
    test_schedule JSONB,                           -- [0, 3, 6, 9, 12, 18, 24, 36]
    
    -- Test methods
    test_methods JSONB,                            -- Array of test_method_ids to perform
    
    -- Status
    status VARCHAR(50) DEFAULT 'active',           -- 'active', 'completed', 'cancelled'
    
    -- Results summary
    results_summary TEXT,
    conclusion TEXT,
    shelf_life_recommendation INT,                 -- Months
    
    -- Approval
    protocol_approved_by INT REFERENCES users(id) ON DELETE SET NULL,
    protocol_approval_date DATE,
    report_approved_by INT REFERENCES users(id) ON DELETE SET NULL,
    report_approval_date DATE,
    
    notes TEXT,
    attachments JSONB,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_stability_studies_number ON stability_studies(study_number);
CREATE INDEX idx_stability_studies_material ON stability_studies(material_id);
CREATE INDEX idx_stability_studies_status ON stability_studies(status);

CREATE TRIGGER trg_update_stability_studies_updated_at
BEFORE UPDATE ON stability_studies
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

-- ============================================================================
-- STABILITY SAMPLES
-- ============================================================================

-- Individual samples for each time point
CREATE TABLE IF NOT EXISTS stability_samples (
    id SERIAL PRIMARY KEY,
    stability_study_id INT NOT NULL REFERENCES stability_studies(id) ON DELETE CASCADE,
    lab_sample_id INT REFERENCES lab_samples(id) ON DELETE SET NULL,
    
    -- Time point
    time_point_months INT NOT NULL,                -- 0, 3, 6, 12, etc.
    scheduled_pull_date DATE NOT NULL,
    actual_pull_date DATE,
    
    -- Testing
    testing_due_date DATE,
    testing_completed BOOLEAN DEFAULT FALSE,
    testing_completed_date DATE,
    
    -- Results
    results_summary JSONB,                         -- Key test results
    all_tests_passed BOOLEAN,
    
    notes TEXT,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_stability_samples_study ON stability_samples(stability_study_id);
CREATE INDEX idx_stability_samples_timepoint ON stability_samples(time_point_months);
CREATE INDEX idx_stability_samples_pull_date ON stability_samples(scheduled_pull_date);

CREATE TRIGGER trg_update_stability_samples_updated_at
BEFORE UPDATE ON stability_samples
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

-- ============================================================================
-- FUNCTIONS & TRIGGERS
-- ============================================================================

-- Generate sample number
CREATE OR REPLACE FUNCTION generate_sample_number()
RETURNS TEXT AS $$
DECLARE
    next_num INT;
    year_part TEXT;
BEGIN
    year_part := TO_CHAR(CURRENT_DATE, 'YYYY');
    SELECT COALESCE(MAX(CAST(SUBSTRING(sample_number FROM 9) AS INT)), 0) + 1
    INTO next_num
    FROM lab_samples
    WHERE sample_number LIKE 'LS-' || year_part || '-%';
    
    RETURN 'LS-' || year_part || '-' || LPAD(next_num::TEXT, 4, '0');
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION set_sample_number()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.sample_number IS NULL OR NEW.sample_number = '' THEN
        NEW.sample_number := generate_sample_number();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_set_sample_number
BEFORE INSERT ON lab_samples
FOR EACH ROW
EXECUTE FUNCTION set_sample_number();

-- Generate CoA number
CREATE OR REPLACE FUNCTION generate_coa_number()
RETURNS TEXT AS $$
DECLARE
    next_num INT;
    year_part TEXT;
BEGIN
    year_part := TO_CHAR(CURRENT_DATE, 'YYYY');
    SELECT COALESCE(MAX(CAST(SUBSTRING(coa_number FROM 10) AS INT)), 0) + 1
    INTO next_num
    FROM certificates_of_analysis
    WHERE coa_number LIKE 'COA-' || year_part || '-%';
    
    RETURN 'COA-' || year_part || '-' || LPAD(next_num::TEXT, 4, '0');
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION set_coa_number()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.coa_number IS NULL OR NEW.coa_number = '' THEN
        NEW.coa_number := generate_coa_number();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_set_coa_number
BEFORE INSERT ON certificates_of_analysis
FOR EACH ROW
EXECUTE FUNCTION set_coa_number();

-- Generate OOS number
CREATE OR REPLACE FUNCTION generate_oos_number()
RETURNS TEXT AS $$
DECLARE
    next_num INT;
    year_part TEXT;
BEGIN
    year_part := TO_CHAR(CURRENT_DATE, 'YYYY');
    SELECT COALESCE(MAX(CAST(SUBSTRING(oos_number FROM 10) AS INT)), 0) + 1
    INTO next_num
    FROM oos_investigations
    WHERE oos_number LIKE 'OOS-' || year_part || '-%';
    
    RETURN 'OOS-' || year_part || '-' || LPAD(next_num::TEXT, 4, '0');
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION set_oos_number()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.oos_number IS NULL OR NEW.oos_number = '' THEN
        NEW.oos_number := generate_oos_number();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_set_oos_number
BEFORE INSERT ON oos_investigations
FOR EACH ROW
EXECUTE FUNCTION set_oos_number();

-- Update equipment calibration status
CREATE OR REPLACE FUNCTION update_equipment_calibration_status()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.next_calibration_date IS NOT NULL THEN
        IF NEW.next_calibration_date < CURRENT_DATE THEN
            NEW.calibration_status := 'overdue';
        ELSIF NEW.next_calibration_date <= CURRENT_DATE + INTERVAL '30 days' THEN
            NEW.calibration_status := 'due_soon';
        ELSE
            NEW.calibration_status := 'calibrated';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_update_equipment_calibration_status
BEFORE INSERT OR UPDATE OF next_calibration_date ON lab_equipment
FOR EACH ROW
EXECUTE FUNCTION update_equipment_calibration_status();

-- ============================================================================
-- VIEWS FOR REPORTING
-- ============================================================================

-- Pending lab work view
CREATE OR REPLACE VIEW v_pending_lab_work AS
SELECT 
    lta.id,
    ls.sample_number,
    ltm.method_code,
    ltm.method_name,
    m.name AS material_name,
    ls.batch_number,
    lta.status,
    lta.priority,
    lta.due_date,
    u.full_name AS assigned_to,
    lta.is_rush,
    CASE 
        WHEN lta.due_date < CURRENT_DATE THEN TRUE 
        ELSE FALSE 
    END AS is_overdue
FROM lab_test_assignments lta
JOIN lab_samples ls ON lta.sample_id = ls.id
JOIN lab_test_methods ltm ON lta.test_method_id = ltm.id
JOIN materials m ON ls.material_id = m.id
LEFT JOIN users u ON lta.assigned_to = u.id
WHERE lta.status IN ('pending', 'in_progress')
ORDER BY lta.is_rush DESC, lta.priority ASC, lta.due_date ASC;

-- Equipment calibration status view
CREATE OR REPLACE VIEW v_equipment_calibration_due AS
SELECT 
    equipment_code,
    equipment_name,
    equipment_type,
    last_calibration_date,
    next_calibration_date,
    calibration_status,
    CASE 
        WHEN next_calibration_date < CURRENT_DATE 
        THEN CURRENT_DATE - next_calibration_date 
        ELSE 0 
    END AS days_overdue,
    location,
    is_operational
FROM lab_equipment
WHERE calibration_status IN ('due_soon', 'overdue')
ORDER BY next_calibration_date ASC;

-- Sample aging report
CREATE OR REPLACE VIEW v_sample_retention_status AS
SELECT 
    sample_number,
    sample_type,
    m.name AS material_name,
    batch_number,
    collection_date,
    retention_expiry_date,
    sample_status,
    storage_location,
    CASE 
        WHEN retention_expiry_date IS NOT NULL AND retention_expiry_date <= CURRENT_DATE + INTERVAL '30 days'
        THEN TRUE 
        ELSE FALSE 
    END AS disposal_due_soon,
    CASE 
        WHEN retention_expiry_date IS NOT NULL AND retention_expiry_date < CURRENT_DATE
        THEN TRUE 
        ELSE FALSE 
    END AS disposal_overdue
FROM lab_samples ls
JOIN materials m ON ls.material_id = m.id
WHERE sample_status IN ('retained', 'completed')
AND retention_required = TRUE
ORDER BY retention_expiry_date ASC;

-- Analyst workload view
CREATE OR REPLACE VIEW v_analyst_workload AS
SELECT 
    u.id AS analyst_id,
    u.full_name AS analyst_name,
    COUNT(lta.id) AS total_assignments,
    COUNT(CASE WHEN lta.status = 'pending' THEN 1 END) AS pending_tests,
    COUNT(CASE WHEN lta.status = 'in_progress' THEN 1 END) AS in_progress_tests,
    COUNT(CASE WHEN lta.is_rush = TRUE THEN 1 END) AS rush_tests,
    COUNT(CASE WHEN lta.due_date < CURRENT_DATE THEN 1 END) AS overdue_tests,
    MIN(lta.due_date) AS earliest_due_date
FROM users u
LEFT JOIN lab_test_assignments lta ON u.id = lta.assigned_to 
    AND lta.status IN ('pending', 'in_progress')
WHERE u.role IN ('admin', 'manager', 'user')
GROUP BY u.id, u.full_name
HAVING COUNT(lta.id) > 0
ORDER BY overdue_tests DESC, rush_tests DESC, total_assignments DESC;

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON TABLE lab_test_methods IS 'Catalog of standard test methods and procedures (ASTM, ISO, internal SOPs)';
COMMENT ON TABLE lab_equipment IS 'Laboratory instruments and equipment with calibration tracking';
COMMENT ON TABLE lab_samples IS 'Physical samples collected for testing with chain of custody';
COMMENT ON TABLE lab_test_assignments IS 'Test work orders linking samples to test methods';
COMMENT ON TABLE lab_test_results IS 'Detailed individual test results and measurements';
COMMENT ON TABLE analyst_qualifications IS 'Track which analysts are trained/qualified for which test methods';
COMMENT ON TABLE certificates_of_analysis IS 'Formal CoA documents issued to customers';
COMMENT ON TABLE oos_investigations IS 'Out-of-specification result investigations (Phase I/II)';
COMMENT ON TABLE stability_studies IS 'Long-term stability studies for shelf-life determination';
COMMENT ON TABLE stability_samples IS 'Individual time-point samples within stability studies';
