-- quotes table
CREATE TABLE quotes (
    -- Identity
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id UUID NOT NULL,
    version INT NOT NULL DEFAULT 1,
    
    -- State management
    state TEXT NOT NULL CHECK (state IN ('draft', 'proposed', 'accepted', 'rejected', 'expired', 'superseded')),
    
    -- Pricing
    amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
    currency TEXT NOT NULL DEFAULT 'NGN',
    breakdown JSONB,
    
    -- Metadata
    notes TEXT,
    estimated_duration_mins INT,
    valid_until TIMESTAMPTZ,
    
    -- Actor tracking
    proposed_by UUID NOT NULL,
    proposed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Decision tracking
    decision_by UUID,
    decided_at TIMESTAMPTZ,
    rejection_reason_code TEXT,
    rejection_reason_text TEXT,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Optimistic locking
    db_version INT NOT NULL DEFAULT 1,
    
    -- Constraints
    CONSTRAINT quotes_booking_id_version_unique UNIQUE (booking_id, version),
    CHECK (
        (state = 'accepted' AND decision_by IS NOT NULL AND decided_at IS NOT NULL) OR
        (state = 'rejected' AND decision_by IS NOT NULL AND decided_at IS NOT NULL AND rejection_reason_code IS NOT NULL) OR
        (state NOT IN ('accepted', 'rejected'))
    )
);

-- Indexes
CREATE INDEX idx_quotes_booking_id ON quotes(booking_id);
CREATE INDEX idx_quotes_proposed_by ON quotes(proposed_by);
CREATE INDEX idx_quotes_state ON quotes(state);
CREATE INDEX idx_quotes_valid_until ON quotes(valid_until) WHERE state = 'proposed';
CREATE INDEX idx_quotes_created_at ON quotes(created_at DESC);

-- Quote price breakdown table
CREATE TABLE quote_price_breakdown (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quote_id UUID NOT NULL REFERENCES quotes(id) ON DELETE CASCADE,
    item_type TEXT NOT NULL CHECK (item_type IN ('labor', 'material', 'travel', 'other')),
    description TEXT NOT NULL,
    quantity DECIMAL(10,2) NOT NULL DEFAULT 1,
    unit_price_cents BIGINT NOT NULL,
    total_cents BIGINT NOT NULL,
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_quote_price_breakdown_quote_id ON quote_price_breakdown(quote_id);