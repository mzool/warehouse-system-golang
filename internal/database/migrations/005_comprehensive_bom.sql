-- Migration 005: Make Bill of Materials more comprehensive
-- This migration adds additional fields to support advanced manufacturing and costing features

-- Add new columns to bills_of_materials table
ALTER TABLE bills_of_materials 
ADD COLUMN IF NOT EXISTS scrap_percentage DECIMAL(5, 2) DEFAULT 0.00 CHECK (scrap_percentage >= 0 AND scrap_percentage <= 100),
ADD COLUMN IF NOT EXISTS fixed_quantity BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS is_optional BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS priority INT DEFAULT 1 CHECK (priority > 0),
ADD COLUMN IF NOT EXISTS reference_designator VARCHAR(100),
ADD COLUMN IF NOT EXISTS notes TEXT,
ADD COLUMN IF NOT EXISTS effective_date DATE,
ADD COLUMN IF NOT EXISTS expiry_date DATE,
ADD COLUMN IF NOT EXISTS version VARCHAR(50) DEFAULT '1.0',
ADD COLUMN IF NOT EXISTS operation_sequence INT,
ADD COLUMN IF NOT EXISTS estimated_cost DECIMAL(15, 2),
ADD COLUMN IF NOT EXISTS actual_cost DECIMAL(15, 2),
ADD COLUMN IF NOT EXISTS lead_time_days INT DEFAULT 0 CHECK (lead_time_days >= 0),
ADD COLUMN IF NOT EXISTS supplier_id INT REFERENCES suppliers(id) ON DELETE SET NULL,
ADD COLUMN IF NOT EXISTS alternate_component_id INT REFERENCES materials(id) ON DELETE SET NULL,
ADD COLUMN IF NOT EXISTS is_active BOOLEAN DEFAULT TRUE,
ADD COLUMN IF NOT EXISTS archived BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS archived_at TIMESTAMP WITH TIME ZONE,
ADD COLUMN IF NOT EXISTS archived_by INT REFERENCES users(id) ON DELETE SET NULL;


-- Add unique constraint to prevent duplicate active BOMs (same finished + component)
CREATE UNIQUE INDEX IF NOT EXISTS idx_bom_unique_active 
ON bills_of_materials(finished_material_id, component_material_id, version) 
WHERE archived = FALSE;

-- Add index for version queries
CREATE INDEX IF NOT EXISTS idx_bom_version ON bills_of_materials(finished_material_id, version);

-- Add index for active BOMs
CREATE INDEX IF NOT EXISTS idx_bom_active ON bills_of_materials(is_active, archived);

-- Add index for effective/expiry date queries
CREATE INDEX IF NOT EXISTS idx_bom_effective_dates ON bills_of_materials(effective_date, expiry_date);

-- Add index for supplier lookups
CREATE INDEX IF NOT EXISTS idx_bom_supplier ON bills_of_materials(supplier_id) WHERE supplier_id IS NOT NULL;

-- Add index for alternate component lookups
CREATE INDEX IF NOT EXISTS idx_bom_alternate ON bills_of_materials(alternate_component_id) WHERE alternate_component_id IS NOT NULL;

-- Add check constraint to ensure effective_date is before expiry_date
ALTER TABLE bills_of_materials 
ADD CONSTRAINT chk_bom_date_range 
CHECK (expiry_date IS NULL OR effective_date IS NULL OR effective_date <= expiry_date);

-- Add trigger to automatically set archived_at when archived flag is set
CREATE OR REPLACE FUNCTION set_bom_archived_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.archived = TRUE AND OLD.archived = FALSE THEN
        NEW.archived_at = CURRENT_TIMESTAMP;
    END IF;
    IF NEW.archived = FALSE THEN
        NEW.archived_at = NULL;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_bom_archive_timestamp
BEFORE UPDATE ON bills_of_materials
FOR EACH ROW
WHEN (NEW.archived IS DISTINCT FROM OLD.archived)
EXECUTE FUNCTION set_bom_archived_timestamp();

-- Add trigger to deactivate expired BOMs automatically
CREATE OR REPLACE FUNCTION check_bom_expiry()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.expiry_date IS NOT NULL AND NEW.expiry_date < CURRENT_DATE THEN
        NEW.is_active = FALSE;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_bom_check_expiry
BEFORE INSERT OR UPDATE ON bills_of_materials
FOR EACH ROW
WHEN (NEW.expiry_date IS NOT NULL)
EXECUTE FUNCTION check_bom_expiry();

-- Create view for effective BOMs (active, not archived, within effective dates)
CREATE OR REPLACE VIEW v_effective_boms AS
SELECT 
    b.*,
    fm.name AS finished_material_name,
    fm.code AS finished_material_code,
    cm.name AS component_material_name,
    cm.code AS component_material_code,
    cm.unit_price AS component_unit_price,
    u.name AS unit_name,
    u.abbreviation AS unit_abbreviation,
    s.name AS supplier_name,
    alt.name AS alternate_component_name,
    alt.code AS alternate_component_code,
    -- Calculate adjusted quantity with scrap
    b.quantity * (1 + (b.scrap_percentage / 100)) AS adjusted_quantity,
    -- Calculate cost if not set
    COALESCE(b.estimated_cost, b.quantity * COALESCE(cm.unit_price, 0)) AS calculated_cost
FROM bills_of_materials b
LEFT JOIN materials fm ON b.finished_material_id = fm.id
LEFT JOIN materials cm ON b.component_material_id = cm.id
LEFT JOIN measure_units u ON b.unit_measure_id = u.id
LEFT JOIN suppliers s ON b.supplier_id = s.id
LEFT JOIN materials alt ON b.alternate_component_id = alt.id
WHERE 
    b.is_active = TRUE 
    AND b.archived = FALSE
    AND (b.effective_date IS NULL OR b.effective_date <= CURRENT_DATE)
    AND (b.expiry_date IS NULL OR b.expiry_date > CURRENT_DATE);

-- Create view for BOM costing with scrap included
CREATE OR REPLACE VIEW v_bom_cost_analysis AS
SELECT 
    b.finished_material_id,
    fm.name AS finished_material_name,
    fm.code AS finished_material_code,
    fm.unit_price AS finished_material_price,
    COUNT(b.id) AS component_count,
    SUM(b.quantity * COALESCE(cm.unit_price, 0)) AS raw_material_cost,
    SUM(b.quantity * (1 + (b.scrap_percentage / 100)) * COALESCE(cm.unit_price, 0)) AS material_cost_with_scrap,
    SUM(COALESCE(b.estimated_cost, b.quantity * COALESCE(cm.unit_price, 0))) AS estimated_total_cost,
    SUM(COALESCE(b.actual_cost, 0)) AS actual_total_cost,
    MAX(b.lead_time_days) AS max_lead_time_days,
    b.version
FROM bills_of_materials b
LEFT JOIN materials fm ON b.finished_material_id = fm.id
LEFT JOIN materials cm ON b.component_material_id = cm.id
WHERE b.is_active = TRUE AND b.archived = FALSE
GROUP BY b.finished_material_id, fm.name, fm.code, fm.unit_price, b.version;

-- Create function to calculate total BOM cost with options
CREATE OR REPLACE FUNCTION calculate_bom_cost(
    p_finished_material_id INT,
    p_include_scrap BOOLEAN DEFAULT TRUE,
    p_use_actual_cost BOOLEAN DEFAULT FALSE,
    p_version VARCHAR(50) DEFAULT NULL
)
RETURNS DECIMAL(15, 2) AS $$
DECLARE
    v_total_cost DECIMAL(15, 2);
BEGIN
    SELECT 
        SUM(
            CASE 
                WHEN p_use_actual_cost AND b.actual_cost IS NOT NULL THEN b.actual_cost
                WHEN b.estimated_cost IS NOT NULL THEN b.estimated_cost
                ELSE 
                    CASE 
                        WHEN p_include_scrap THEN 
                            b.quantity * (1 + (b.scrap_percentage / 100)) * COALESCE(cm.unit_price, 0)
                        ELSE 
                            b.quantity * COALESCE(cm.unit_price, 0)
                    END
            END
        )
    INTO v_total_cost
    FROM bills_of_materials b
    LEFT JOIN materials cm ON b.component_material_id = cm.id
    WHERE 
        b.finished_material_id = p_finished_material_id
        AND b.is_active = TRUE
        AND b.archived = FALSE
        AND (p_version IS NULL OR b.version = p_version);
    
    RETURN COALESCE(v_total_cost, 0);
END;
$$ LANGUAGE plpgsql;

-- Create function to get material requirements with scrap
CREATE OR REPLACE FUNCTION calculate_material_requirements(
    p_finished_material_id INT,
    p_production_quantity DECIMAL(15, 4),
    p_include_scrap BOOLEAN DEFAULT TRUE
)
RETURNS TABLE (
    component_material_id INT,
    component_name VARCHAR,
    component_code VARCHAR,
    base_quantity DECIMAL(15, 4),
    scrap_percentage DECIMAL(5, 2),
    adjusted_quantity DECIMAL(15, 4),
    total_required DECIMAL(15, 4),
    unit_name VARCHAR,
    is_fixed_quantity BOOLEAN,
    lead_time_days INT,
    supplier_name VARCHAR
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        b.component_material_id,
        m.name AS component_name,
        m.code AS component_code,
        b.quantity AS base_quantity,
        b.scrap_percentage,
        CASE 
            WHEN p_include_scrap THEN b.quantity * (1 + (b.scrap_percentage / 100))
            ELSE b.quantity
        END AS adjusted_quantity,
        CASE 
            WHEN b.fixed_quantity THEN 
                CASE 
                    WHEN p_include_scrap THEN b.quantity * (1 + (b.scrap_percentage / 100))
                    ELSE b.quantity
                END
            ELSE 
                CASE 
                    WHEN p_include_scrap THEN b.quantity * (1 + (b.scrap_percentage / 100)) * p_production_quantity
                    ELSE b.quantity * p_production_quantity
                END
        END AS total_required,
        u.name AS unit_name,
        b.fixed_quantity AS is_fixed_quantity,
        b.lead_time_days,
        s.name AS supplier_name
    FROM bills_of_materials b
    LEFT JOIN materials m ON b.component_material_id = m.id
    LEFT JOIN measure_units u ON b.unit_measure_id = u.id
    LEFT JOIN suppliers s ON b.supplier_id = s.id
    WHERE 
        b.finished_material_id = p_finished_material_id
        AND b.is_active = TRUE
        AND b.archived = FALSE
    ORDER BY b.priority, b.operation_sequence NULLS LAST;
END;
$$ LANGUAGE plpgsql;

-- Add sample data update to set default values for existing records
UPDATE bills_of_materials 
SET 
    scrap_percentage = 0.00,
    fixed_quantity = FALSE,
    is_optional = FALSE,
    priority = 1,
    version = '1.0',
    lead_time_days = 0,
    is_active = TRUE,
    archived = FALSE
WHERE 
    scrap_percentage IS NULL 
    OR fixed_quantity IS NULL 
    OR is_optional IS NULL 
    OR priority IS NULL 
    OR version IS NULL 
    OR lead_time_days IS NULL
    OR is_active IS NULL
    OR archived IS NULL;
