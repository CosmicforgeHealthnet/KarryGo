CREATE TABLE delivery_proofs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id UUID NOT NULL UNIQUE REFERENCES trips(id) ON DELETE CASCADE,
    photo_ref TEXT NOT NULL,
    signature_ref TEXT NOT NULL,
    receiver_name TEXT NOT NULL,
    receiver_phone TEXT NOT NULL,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    verified BOOLEAN NOT NULL DEFAULT false,
    verified_at TIMESTAMPTZ NULL
);

CREATE UNIQUE INDEX idx_proof_trip ON delivery_proofs (trip_id);
