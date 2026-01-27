-- Migration: Change foreign key constraints to RESTRICT instead of SET NULL
-- Purpose: Prevent accidental deletion of units/categories that are still in use
-- 
-- This ensures data integrity by:
-- 1. Preventing deletion of units that other units reference (convert_to)
-- 2. Preventing deletion of units that materials are using (measure_unit_id)
-- 3. Preventing deletion of categories that materials are using (category)
--
-- With RESTRICT: Database will reject the DELETE and return an error
-- With SET NULL (old): Database would allow DELETE and set references to NULL (data loss)

-- ============================================================================
-- MEASURE UNITS: Fix convert_to foreign key
-- ============================================================================

-- Drop the existing foreign key constraint for convert_to
ALTER TABLE measure_units
DROP CONSTRAINT IF EXISTS measure_units_convert_to_fkey;

-- Add it back with RESTRICT to prevent deletion of base units
-- If a unit is referenced by other units, deletion will fail with clear error
ALTER TABLE measure_units
ADD CONSTRAINT measure_units_convert_to_fkey 
    FOREIGN KEY (convert_to) 
    REFERENCES measure_units(id) 
    ON DELETE RESTRICT;

-- ============================================================================
-- MATERIALS: Fix measure_unit_id foreign key
-- ============================================================================

-- Drop the existing foreign key constraint for measure_unit_id
ALTER TABLE materials
DROP CONSTRAINT IF EXISTS materials_measure_unit_id_fkey;

-- Add it back with RESTRICT to prevent deletion of units in use
-- If a unit is used by any material, deletion will fail
ALTER TABLE materials
ADD CONSTRAINT materials_measure_unit_id_fkey 
    FOREIGN KEY (measure_unit_id) 
    REFERENCES measure_units(id) 
    ON DELETE RESTRICT;

-- ============================================================================
-- MATERIALS: Fix category foreign key
-- ============================================================================

-- Drop the existing foreign key constraint for category
ALTER TABLE materials
DROP CONSTRAINT IF EXISTS materials_category_fkey;

-- Add it back with RESTRICT to prevent deletion of categories in use
-- If a category is used by any material, deletion will fail
ALTER TABLE materials
ADD CONSTRAINT materials_category_fkey 
    FOREIGN KEY (category) 
    REFERENCES material_categories(id) 
    ON DELETE RESTRICT;

-- ============================================================================
-- Notes:
-- ============================================================================
-- 
-- After this migration:
-- - You CANNOT delete a unit if other units convert to it
-- - You CANNOT delete a unit if materials use it
-- - You CANNOT delete a category if materials use it
-- - You MUST first update/delete the dependent records
--
-- This is safer than SET NULL because:
-- - No data loss (references don't get nullified)
-- - Clear error messages at database level
-- - Forces proper cleanup workflow
-- - API layer can provide user-friendly error messages
--
-- Example error message from PostgreSQL:
-- ERROR: update or delete on table "measure_units" violates foreign key 
-- constraint "materials_measure_unit_id_fkey" on table "materials" 