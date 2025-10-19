-- Create bookings table
--encore:service=booking
create table bookings (
    id uuid primary key default generate_uuid(),
    customer_id uuid not null, -- Reference to customer service 
    artisan_id uuid,           -- Reference to artisan service
    service_category_id text not null, -- Reference to service categories
    title text not null check (length(title) >= 5 and length(title) <= 100),
    description text check (length(description) >= 20),
    customer_address_id uuid not null, -- Reference to customer service 
    status varchar(20) not null default 'pending_payment',
    priority varchar(10) default 'normal',
    scheduled_at timestamptz,
    estimated_duration_mins integer check (estimated_duration_mins > 0 and estimated_duration_mins <= 1440), -- 1 day max
    metadata jsonb, -- For future extensibility
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    deleted_at timestamptz,
    version bigint not null default 1, -- For optimistic locking

    -- Constraints
    constraint valid_status check (
        status in ('requested', 'assigned', 'payment_pending', 'confirmed', 'enroute', 'in_progress', 'completed', 'cancelled', 'closed')
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

-- Indexes for performance
create index IF NOT EXISTS idx_bookings_customer on bookings (customer_id, created_at desc);
create index IF NOT EXISTS idx_bookings_artisan on bookings (artisan_id, created_at desc);
CREATE INDEX IF NOT EXISTS idx_bookings_status ON bookings(status);
CREATE INDEX IF NOT EXISTS idx_bookings_created_at ON bookings(created_at);
