-- ShedLock's own required table (not a JPA entity, so ddl-auto=update won't create it).
-- Spring Boot runs schema.sql automatically on every startup; IF NOT EXISTS makes this safe
-- to re-run every time without affecting an already-existing table.
CREATE TABLE IF NOT EXISTS shedlock (
    name VARCHAR(64) NOT NULL,
    lock_until TIMESTAMP(3) NOT NULL,
    locked_at TIMESTAMP(3) NOT NULL,
    locked_by VARCHAR(255) NOT NULL,
    PRIMARY KEY (name)
);
