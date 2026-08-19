-- BidCraft schema: users, auctions, bids.
-- Money is stored as integer cents (BIGINT) to keep accept/reject comparisons exact.
-- Status is TEXT + CHECK rather than a native ENUM (simpler migrations, identical integrity).

CREATE TABLE users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT        NOT NULL UNIQUE,
    username      TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT users_email_lower_ck  CHECK (email = lower(email)),
    CONSTRAINT users_username_len_ck CHECK (char_length(username) BETWEEN 3 AND 32)
);

CREATE TABLE auctions (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    seller_id           UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    title               TEXT        NOT NULL,
    description         TEXT        NOT NULL DEFAULT '',
    image_url           TEXT        NOT NULL,
    base_price_cents    BIGINT      NOT NULL,
    current_price_cents BIGINT      NOT NULL,
    min_increment_cents BIGINT      NOT NULL,
    current_winner_id   UUID            NULL REFERENCES users(id) ON DELETE SET NULL,
    bid_count           INTEGER     NOT NULL DEFAULT 0,
    status              TEXT        NOT NULL DEFAULT 'created',
    starts_at           TIMESTAMPTZ NOT NULL,
    ends_at             TIMESTAMPTZ NOT NULL,
    closed_at           TIMESTAMPTZ     NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT auctions_status_ck      CHECK (status IN ('created', 'active', 'finished')),
    CONSTRAINT auctions_window_ck      CHECK (ends_at > starts_at),
    CONSTRAINT auctions_base_ck        CHECK (base_price_cents >= 0),
    CONSTRAINT auctions_increment_ck   CHECK (min_increment_cents > 0),
    CONSTRAINT auctions_price_floor_ck CHECK (current_price_cents >= base_price_cents),
    CONSTRAINT auctions_title_ck       CHECK (char_length(btrim(title)) BETWEEN 3 AND 140),
    -- A finished auction must record its closing instant; a winner implies at least one bid.
    CONSTRAINT auctions_closed_ck      CHECK ((status = 'finished') = (closed_at IS NOT NULL)),
    CONSTRAINT auctions_winner_ck      CHECK ((current_winner_id IS NULL) = (bid_count = 0))
);

CREATE TABLE bids (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    seq          BIGINT      GENERATED ALWAYS AS IDENTITY,
    auction_id   UUID        NOT NULL REFERENCES auctions(id) ON DELETE CASCADE,
    bidder_id    UUID        NOT NULL REFERENCES users(id)    ON DELETE RESTRICT,
    amount_cents BIGINT      NOT NULL CHECK (amount_cents > 0),
    placed_at    TIMESTAMPTZ NOT NULL,

    -- Accepted bids on an auction are strictly increasing, so two equal amounts
    -- can never both be accepted. This is a tripwire: if a race ever slipped past
    -- the engine AND the conditional UPDATE, the database itself refuses it.
    CONSTRAINT bids_amount_unique UNIQUE (auction_id, amount_cents)
);

CREATE INDEX idx_auctions_status_ends ON auctions (status, ends_at DESC);
CREATE INDEX idx_auctions_seller      ON auctions (seller_id);
CREATE INDEX idx_auctions_open        ON auctions (ends_at) WHERE status IN ('created', 'active');
CREATE INDEX idx_bids_auction_seq     ON bids (auction_id, seq DESC);
