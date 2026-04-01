CREATE TABLE IF NOT EXISTS customers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    name TEXT NOT NULL,
    email TEXT,
    phone TEXT,
    address TEXT,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_customers_organization_name
    ON customers (organization_id, name)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_customers_organization_created_at
    ON customers (organization_id, created_at DESC)
    WHERE deleted_at IS NULL;
