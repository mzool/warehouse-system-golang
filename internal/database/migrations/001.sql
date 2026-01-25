CREATE EXTENSION IF NOT EXISTS "pg_trgm";


--- materials types
CREATE TYPE material_type AS ENUM ('raw', 'intermediate', 'finished', 'consumable', 'service');

--- valution type
CREATE TYPE VALUATION_METHOD AS ENUM ('FIFO', 'LIFO', 'Weighted Average');
-- trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;


--- users roles type --> I will keep it simple for now (admin --> full access, manager --> manage materials and warehouses, user --> read-only)
CREATE TYPE user_role AS ENUM ('admin', 'manager', 'user');
--- users table --- this system is about materials management so user management will be basic
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(100) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    full_name VARCHAR(255),
    role user_role NOT NULL DEFAULT 'user',
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER trg_update_users_updated_at
BEFORE UPDATE ON users
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

--- measure units table
CREATE TABLE IF NOT EXISTS measure_units (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    abbreviation VARCHAR(20) NOT NULL UNIQUE,
    convertion_factor DECIMAL(10, 4) NOT NULL DEFAULT 1.0,
    convert_to INT REFERENCES measure_units(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_measure_units_name_trgm ON measure_units USING GIN (name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_measure_units_abbreviation_trgm ON measure_units USING GIN (abbreviation gin_trgm_ops);
CREATE TRIGGER trg_update_measure_units_updated_at
BEFORE UPDATE ON measure_units
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

--- materials categories table
CREATE TABLE IF NOT EXISTS material_categories (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    meta JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_material_categories_name_trgm ON material_categories USING GIN (name gin_trgm_ops);
CREATE TRIGGER trg_update_material_categories_updated_at
BEFORE UPDATE ON material_categories
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

--- materials table
CREATE TABLE IF NOT EXISTS materials (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    valuation VALUATION_METHOD,
    --- classification fields
    type material_type NOT NULL,
    saleable BOOLEAN DEFAULT TRUE,
    unit_price DECIMAL(15, 4),
    sale_price DECIMAL(15, 4),
    category INT REFERENCES material_categories(id) ON DELETE SET NULL,
    code VARCHAR(100) UNIQUE NOT NULL,
    sku VARCHAR(100) UNIQUE NOT NULL,
    barcode VARCHAR(100) UNIQUE,
    --- physical properties
    measure_unit_id INT REFERENCES measure_units(id) ON DELETE SET NULL,
    weight DECIMAL(10, 4),
    volume DECIMAL(10, 4),
    density DECIMAL(10, 4),
    is_toxic BOOLEAN DEFAULT FALSE,
    is_flammable BOOLEAN DEFAULT FALSE,
    is_fragile BOOLEAN DEFAULT FALSE,
    -- files
    image_url VARCHAR(500),
    document_url VARCHAR(500),
    --- finance fields
    tax_rate DECIMAL(5, 2) DEFAULT 0.0,
    discount_rate DECIMAL(5, 2) DEFAULT 0.0,
    --- status fields
    is_active BOOLEAN DEFAULT TRUE,
    archived BOOLEAN DEFAULT FALSE,
    --- metadata
    meta JSONB,
    -- timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_materials_description_trgm ON materials USING GIN (description gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_materials_category ON materials(category);
CREATE INDEX IF NOT EXISTS idx_materials_type ON materials(type);
CREATE INDEX IF NOT EXISTS idx_materials_is_active ON materials(is_active) WHERE is_active = TRUE;
CREATE INDEX IF NOT EXISTS idx_materials_archived ON materials(archived) WHERE archived = FALSE;
CREATE TRIGGER trg_update_materials_updated_at
BEFORE UPDATE ON materials
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

--- warehouses table --- wharehouse here is the physical location where materials are stored so it not about stock levels
CREATE TABLE IF NOT EXISTS warehouses (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    code VARCHAR(100) UNIQUE NOT NULL,
    location VARCHAR(500),
    description TEXT,
    valuation VALUATION_METHOD NOT NULL DEFAULT 'FIFO', -- e.g., FIFO, LIFO, Weighted Average
    parent_warehouse INT REFERENCES warehouses(id) ON DELETE SET NULL, -- self-referencing for hierarchical warehouses
    capacity DECIMAL(15, 4),
    meta JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE TRIGGER trg_update_warehouses_updated_at
BEFORE UPDATE ON warehouses
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

--- suppliers table
CREATE TABLE IF NOT EXISTS suppliers (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    contact_name VARCHAR(255) UNIQUE,
    contact_email VARCHAR(255) UNIQUE,
    contact_phone VARCHAR(50) UNIQUE,
    address TEXT,
    meta JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_suppliers_address_trgm ON suppliers USING GIN (address gin_trgm_ops);
CREATE TRIGGER trg_update_suppliers_updated_at
BEFORE UPDATE ON suppliers
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

--- customers table
CREATE TABLE IF NOT EXISTS customers (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    contact_name VARCHAR(255) UNIQUE,
    contact_email VARCHAR(255) UNIQUE,
    contact_phone VARCHAR(50) UNIQUE,
    address TEXT,
    meta JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_customers_address_trgm ON customers USING GIN (address gin_trgm_ops);
CREATE TRIGGER trg_update_customers_updated_at
BEFORE UPDATE ON customers
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

CREATE TYPE stock_direction AS ENUM ('IN', 'OUT');

CREATE TYPE stock_movement_type AS ENUM (
  'OPENING',
  'PURCHASE_RECEIPT',
  'SALE',
  'CUSTOMER_RETURN',
  'TRANSFER_IN',
  'TRANSFER_OUT',
  'SCRAP',
  'ADJUSTMENT_IN',
  'ADJUSTMENT_OUT'
);
--- stock movements table
CREATE TABLE IF NOT EXISTS stock_movements (
    id SERIAL PRIMARY KEY,
    material_id INT REFERENCES materials(id) ON DELETE CASCADE,
    from_warehouse_id INT REFERENCES warehouses(id) ON DELETE SET NULL,
    to_warehouse_id INT REFERENCES warehouses(id) ON DELETE SET NULL,
    quantity DECIMAL(15, 4) NOT NULL,
    stock_direction stock_direction NOT NULL,
    movement_type stock_movement_type NOT NULL,
    reference VARCHAR(255), -- e.g., purchase order number, sales order number
    performed_by INT REFERENCES users(id) ON DELETE SET NULL,
    movement_date TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_stock_movements_material_id ON stock_movements(material_id);
CREATE INDEX IF NOT EXISTS idx_stock_movements_from_warehouse_id ON stock_movements(from_warehouse_id);
CREATE INDEX IF NOT EXISTS idx_stock_movements_to_warehouse_id ON stock_movements(to_warehouse_id);
CREATE INDEX IF NOT EXISTS idx_stock_movements_movement_type ON stock_movements(movement_type);
CREATE INDEX IF NOT EXISTS idx_stock_movements_movement_date ON stock_movements(movement_date);
CREATE INDEX IF NOT EXISTS idx_stock_movements_performed_by ON stock_movements(performed_by);
CREATE TRIGGER trg_update_stock_movements_updated_at
BEFORE UPDATE ON stock_movements
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

--- BATCHES table
CREATE TABLE IF NOT EXISTS batches (
    id SERIAL PRIMARY KEY,
    material_id INT REFERENCES materials(id) ON DELETE CASCADE,
    supplier_id INT REFERENCES suppliers(id) ON DELETE SET NULL,
    warehouse_id INT REFERENCES warehouses(id) ON DELETE SET NULL,
    movement_id INT REFERENCES stock_movements(id) ON DELETE SET NULL,
    unit_price DECIMAL(15, 4),
    batch_number VARCHAR(100) NOT NULL,
    manufacture_date DATE,
    expiry_date DATE,
    start_quantity DECIMAL(15, 4) NOT NULL, --- initial quantity when batch is created 
    current_quantity DECIMAL(15, 4) NOT NULL, --- current quantity in the batch change with stock movements
    notes TEXT,
    meta JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_batches_material_id ON batches(material_id);
CREATE INDEX IF NOT EXISTS idx_batches_supplier_id ON batches(supplier_id);
CREATE INDEX IF NOT EXISTS idx_batches_warehouse_id ON batches(warehouse_id);
CREATE INDEX IF NOT EXISTS idx_batches_movement_id ON batches(movement_id);
CREATE INDEX IF NOT EXISTS idx_batches_batch_number_trgm ON batches USING GIN (batch_number gin_trgm_ops);
CREATE TRIGGER trg_update_batches_updated_at
BEFORE UPDATE ON batches
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();


--- purchase orders table
CREATE TABLE IF NOT EXISTS purchase_orders (
    id SERIAL PRIMARY KEY,
    order_number VARCHAR(100) NOT NULL UNIQUE,
    supplier_id INT REFERENCES suppliers(id) ON DELETE SET NULL,
    order_date TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    expected_delivery_date TIMESTAMP WITH TIME ZONE,
    status VARCHAR(50) NOT NULL DEFAULT 'Pending', -- e.g., Pending, Received, Cancelled
    total_amount DECIMAL(15, 4) NOT NULL,
    created_by INT REFERENCES users(id) ON DELETE SET NULL,
    approved_by INT REFERENCES users(id) ON DELETE SET NULL,
    meta JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_purchase_orders_supplier_id ON purchase_orders(supplier_id);
CREATE INDEX IF NOT EXISTS idx_purchase_orders_status ON purchase_orders(status);
CREATE INDEX IF NOT EXISTS idx_purchase_orders_created_by ON purchase_orders(created_by);
CREATE INDEX IF NOT EXISTS idx_purchase_orders_approved_by ON purchase_orders(approved_by);
CREATE TRIGGER trg_update_purchase_orders_updated_at
BEFORE UPDATE ON purchase_orders
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

--- purchase order items table
CREATE TABLE IF NOT EXISTS purchase_order_items (
    id SERIAL PRIMARY KEY,
    purchase_order_id INT REFERENCES purchase_orders(id) ON DELETE CASCADE,
    material_id INT REFERENCES materials(id) ON DELETE SET NULL,
    quantity DECIMAL(15, 4) NOT NULL,
    unit_price DECIMAL(15, 4) NOT NULL,
    total_price DECIMAL(15, 4) NOT NULL,
    received_quantity DECIMAL(15, 4) DEFAULT 0.0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_purchase_order_items_purchase_order_id ON purchase_order_items(purchase_order_id);
CREATE INDEX IF NOT EXISTS idx_purchase_order_items_material_id ON purchase_order_items(material_id);
CREATE TRIGGER trg_update_purchase_order_items_updated_at
BEFORE UPDATE ON purchase_order_items
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();


--- sales orders table
CREATE TABLE IF NOT EXISTS sales_orders (
    id SERIAL PRIMARY KEY,
    order_number VARCHAR(100) NOT NULL UNIQUE,
    customer_id INT REFERENCES customers(id) ON DELETE SET NULL,
    order_date TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    expected_delivery_date TIMESTAMP WITH TIME ZONE,
    status VARCHAR(50) NOT NULL DEFAULT 'Pending', -- e.g., Pending, Shipped, Cancelled
    total_amount DECIMAL(15, 4) NOT NULL,
    created_by INT REFERENCES users(id) ON DELETE SET NULL,
    approved_by INT REFERENCES users(id) ON DELETE SET NULL,
    meta JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_sales_orders_customer_id ON sales_orders(customer_id);
CREATE INDEX IF NOT EXISTS idx_sales_orders_status ON sales_orders(status);
CREATE INDEX IF NOT EXISTS idx_sales_orders_created_by ON sales_orders(created_by);
CREATE INDEX IF NOT EXISTS idx_sales_orders_approved_by ON sales_orders(approved_by);
CREATE TRIGGER trg_update_sales_orders_updated_at
BEFORE UPDATE ON sales_orders
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

--- sales order items table
CREATE TABLE IF NOT EXISTS sales_order_items (
    id SERIAL PRIMARY KEY,
    sales_order_id INT REFERENCES sales_orders(id) ON DELETE CASCADE,
    material_id INT REFERENCES materials(id) ON DELETE SET NULL,
    quantity DECIMAL(15, 4) NOT NULL,
    unit_price DECIMAL(15, 4) NOT NULL,
    total_price DECIMAL(15, 4) NOT NULL,
    shipped_quantity DECIMAL(15, 4) DEFAULT 0.0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_sales_order_items_sales_order_id ON sales_order_items(sales_order_id);
CREATE INDEX IF NOT EXISTS idx_sales_order_items_material_id ON sales_order_items(material_id);
CREATE TRIGGER trg_update_sales_order_items_updated_at
BEFORE UPDATE ON sales_order_items
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

--- bom table
CREATE TABLE IF NOT EXISTS bills_of_materials (
    id SERIAL PRIMARY KEY,
    finished_material_id INT REFERENCES materials(id) ON DELETE CASCADE,
    component_material_id INT REFERENCES materials(id) ON DELETE SET NULL,
    quantity DECIMAL(15, 4) NOT NULL,
    unit_measure_id INT REFERENCES measure_units(id) ON DELETE SET NULL,
    meta JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_bom_finished_material_id ON bills_of_materials(finished_material_id);
CREATE INDEX IF NOT EXISTS idx_bom_component_material_id ON bills_of_materials(component_material_id);
CREATE TRIGGER trg_update_bom_updated_at
BEFORE UPDATE ON bills_of_materials
FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

