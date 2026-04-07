CREATE TABLE IF NOT EXISTS work_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    work_order_id TEXT NOT NULL,
    customer_name TEXT NOT NULL,
    customer_email TEXT,
    customer_phone TEXT,
    customer_address TEXT NOT NULL,
    job_date DATE NOT NULL,
    status TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_work_orders_organization_work_order_id
    ON work_orders (organization_id, work_order_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_work_orders_organization_job_date_created_at
    ON work_orders (organization_id, job_date DESC, created_at DESC)
    WHERE deleted_at IS NULL;
