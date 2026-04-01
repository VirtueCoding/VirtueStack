SET lock_timeout = '5s';

-- Sequential invoice counter for gap-free numbering.
-- One row per prefix; atomic increment via UPDATE ... RETURNING.
CREATE TABLE IF NOT EXISTS billing_invoice_counters (
    prefix      VARCHAR(10) PRIMARY KEY,
    last_number INTEGER NOT NULL DEFAULT 0
);

INSERT INTO billing_invoice_counters (prefix, last_number)
VALUES ('INV', 0)
ON CONFLICT (prefix) DO NOTHING;

-- Invoices generated from billing transactions.
CREATE TABLE IF NOT EXISTS billing_invoices (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id  UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    invoice_number VARCHAR(30) NOT NULL UNIQUE,
    period_start TIMESTAMPTZ NOT NULL,
    period_end   TIMESTAMPTZ NOT NULL,
    subtotal     BIGINT NOT NULL,
    tax_amount   BIGINT NOT NULL DEFAULT 0,
    total        BIGINT NOT NULL,
    currency     VARCHAR(3) NOT NULL DEFAULT 'USD',
    status       VARCHAR(20) NOT NULL DEFAULT 'draft'
                 CHECK (status IN ('draft', 'issued', 'paid', 'void')),
    line_items   JSONB NOT NULL DEFAULT '[]',
    issued_at    TIMESTAMPTZ,
    paid_at      TIMESTAMPTZ,
    pdf_path     VARCHAR(500),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_billing_invoices_customer ON billing_invoices(customer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_billing_invoices_number ON billing_invoices(invoice_number);
CREATE INDEX IF NOT EXISTS idx_billing_invoices_status ON billing_invoices(status);
CREATE INDEX IF NOT EXISTS idx_billing_invoices_period ON billing_invoices(period_start, period_end);

-- RLS: customers can only read their own invoices.
ALTER TABLE billing_invoices ENABLE ROW LEVEL SECURITY;

CREATE POLICY billing_invoices_customer_policy ON billing_invoices
    FOR SELECT TO PUBLIC
    USING (customer_id = current_setting('app.current_customer_id')::UUID);
