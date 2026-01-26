CREATE OR REPLACE FUNCTION check_units_consistency()
RETURNS TRIGGER AS $$
BEGIN
    -- Check if convert_to is not same as the unit itself on update
    -- and the conversion factor is not 1 then prevent
    IF NEW.convert_to = NEW.id AND NEW.convertion_factor <> 1 THEN
        RAISE EXCEPTION 'Conversion factor must be 1 when convert_to is the same as the unit itself.';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_check_units_consistency
BEFORE INSERT OR UPDATE ON measure_units
FOR EACH ROW
EXECUTE FUNCTION check_units_consistency();
