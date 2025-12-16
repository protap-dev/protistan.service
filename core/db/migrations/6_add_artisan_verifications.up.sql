-- Migration 6: Add artisan_verifications table and availability_status to artisans
-- This migration replaces the single 'verified' boolean with a proper verification table

-- Create the artisan_verifications table to track verification status with history
CREATE TABLE artisan_verifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    artisan_id UUID NOT NULL REFERENCES artisans(id) ON DELETE CASCADE,
    verification_status VARCHAR(20) NOT NULL DEFAULT 'unverified',
    verification_method VARCHAR(50),
    verified_by UUID REFERENCES users(id),
    verification_notes TEXT,
    verification_updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Ensure one verification record per artisan
    CONSTRAINT unique_artisan_verification UNIQUE (artisan_id)
);

-- Create indexes for performance
CREATE INDEX idx_artisan_verifications_artisan_id ON artisan_verifications(artisan_id);
CREATE INDEX idx_artisan_verifications_status ON artisan_verifications(verification_status);
CREATE INDEX idx_artisan_verifications_updated_at ON artisan_verifications(verification_updated_at);

-- Add check constraints for valid statuses
ALTER TABLE artisan_verifications 
ADD CONSTRAINT chk_verification_status 
CHECK (verification_status IN ('unverified', 'pending', 'verified', 'rejected', 'suspended'));

-- Add comments for documentation
COMMENT ON TABLE artisan_verifications IS 'Tracks verification status and history for artisans';
COMMENT ON COLUMN artisan_verifications.verification_status IS 'Current verification status: unverified, pending, verified, rejected, suspended';
COMMENT ON COLUMN artisan_verifications.verification_method IS 'How verification was completed: email, phone, document, manual';
COMMENT ON COLUMN artisan_verifications.verified_by IS 'Admin who verified this artisan';
COMMENT ON COLUMN artisan_verifications.verification_notes IS 'Notes about verification process';
COMMENT ON COLUMN artisans.availability_status IS 'Current availability: available, busy, on_leave, unavailable, suspended';
