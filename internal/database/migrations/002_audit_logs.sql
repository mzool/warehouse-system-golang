CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL,
    user_id INT REFERENCES users(id) ON DELETE SET NULL,
    username VARCHAR(100),
    action VARCHAR(100) NOT NULL,
    entity VARCHAR(100) NOT NULL,
    entity_id INT,
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    details JSONB,
    PRIMARY KEY (id, timestamp)
) PARTITION BY RANGE (timestamp);
