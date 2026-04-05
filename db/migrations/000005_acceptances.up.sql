CREATE TABLE IF NOT EXISTS acceptances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    work_order_id TEXT NOT NULL,
    customer_name TEXT NOT NULL,
    customer_email TEXT NOT NULL,
    service_date TEXT NOT NULL,
    service_expiration_date TEXT NOT NULL,
    service_type TEXT NOT NULL,
    products TEXT[] NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    approved BOOLEAN NOT NULL,
    signature_image_base64 TEXT NOT NULL,
    signed_at TIMESTAMPTZ NOT NULL,
    signed_by_technician_id TEXT NOT NULL,
    pdf_status TEXT NOT NULL,
    email_status TEXT NOT NULL,
    pdf_storage_key TEXT,
    pdf_mime_type TEXT,
    pdf_error TEXT,
    pdf_generated_at TIMESTAMPTZ,
    email_sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT acceptances_org_work_order_unique UNIQUE (organization_id, work_order_id),
    CONSTRAINT acceptances_pdf_status_check CHECK (pdf_status IN ('pending', 'generated', 'failed')),
    CONSTRAINT acceptances_email_status_check CHECK (email_status IN ('pending', 'sent', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_acceptances_organization_created_at
    ON acceptances (organization_id, created_at DESC);
