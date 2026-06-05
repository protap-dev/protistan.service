-- Create bookings table
--encore:service=booking
create table bookings (
    id uuid primary key default generate_uuid(),
    customer_id uuid not null, -- Reference to customer service 
    artisan_id uuid,           -- Reference to artisan service
    service_category_id text not null, -- Reference to service categories
    title text not null check (length(title) >= 5 and length(title) <= 100),
    service_id uuid,                   -- Reference to service item
    media_urls text[],
    is_flexible boolean DEFAULT false,
    description text check (length(description) >= 20),
    customer_address_id uuid not null, -- Reference to customer service 
    status varchar(20) not null default 'pending_payment',
    priority varchar(10) default 'normal',
    scheduled_at timestamptz,

    metadata jsonb, -- For future extensibility
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    deleted_at timestamptz,
    version bigint not null default 1, -- For optimistic locking

    -- Constraints
    constraint valid_status check (
    status IN (
        'requested',
        'offer_pending',
        'offer_rejected',
        'assigned',
        'pending_quote',
        'quote_proposed',
        'quote_accepted',
        'quote_rejected',
        'payment_pending',
        'confirmed',
        'enroute',
        'in_progress',
        'completion_pending',
        'completed',
        'cancelled',
        'closed'
    )
),
    constraint valid_priority check (
        priority in ('low', 'normal', 'high', 'urgent')
    )
);

-- Create booking status history for audit trail
CREATE TABLE IF NOT EXISTS booking_status_history (
    id uuid primary key default generate_uuid(),
    booking_id uuid not null references bookings(id),
    status varchar(20) not null,
    previous_status varchar(20),
    changed_by uuid, -- Reference to user service
    reason text,
    created_at timestamptz default now()
);

-- Create idempotency table history for idempotency handling
CREATE TABLE IF NOT EXISTS idempotency_keys (
    idempotency_key VARCHAR(255) PRIMARY KEY,
    user_id UUID NOT NULL,
    request_hash TEXT NOT NULL, -- Hash of request body
    booking_id UUID, -- Result reference
    response_body JSONB, -- Cached response
    status VARCHAR(20) NOT NULL DEFAULT 'processing', -- processing, completed, failed
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL
);

-- Indexes for performance
create index IF NOT EXISTS idx_bookings_customer on bookings (customer_id, created_at desc);
create index IF NOT EXISTS idx_bookings_artisan on bookings (artisan_id, created_at desc);
CREATE INDEX IF NOT EXISTS idx_bookings_status ON bookings(status);
CREATE INDEX IF NOT EXISTS idx_bookings_created_at ON bookings(created_at);
CREATE INDEX idx_idempotency_keys_expires_at ON idempotency_keys(expires_at);
CREATE INDEX idx_idempotency_keys_user_id ON idempotency_keys(user_id);
