SET lock_timeout = '5s';

CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID REFERENCES customers(id) ON DELETE CASCADE,
    admin_id UUID REFERENCES admins(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL,
    title VARCHAR(255) NOT NULL,
    message TEXT NOT NULL,
    data JSONB NOT NULL DEFAULT '{}',
    read BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notifications_one_recipient_ck
        CHECK (
            (customer_id IS NOT NULL AND admin_id IS NULL) OR
            (customer_id IS NULL AND admin_id IS NOT NULL)
        )
);

CREATE INDEX idx_notifications_customer_unread
    ON notifications(customer_id, created_at DESC) WHERE NOT read;
CREATE INDEX idx_notifications_admin_unread
    ON notifications(admin_id, created_at DESC) WHERE NOT read;
CREATE INDEX idx_notifications_customer_created
    ON notifications(customer_id, created_at DESC);
CREATE INDEX idx_notifications_admin_created
    ON notifications(admin_id, created_at DESC);

ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;

CREATE POLICY notifications_customer_policy ON notifications
    FOR ALL TO PUBLIC
    USING (customer_id = current_setting('app.current_customer_id', true)::UUID);
