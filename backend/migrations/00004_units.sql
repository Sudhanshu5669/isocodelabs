-- Individual flats for the public catalogue. Each one points at a floor plan
-- (`layout`) that the website draws as a furnished 3D model.
--
-- The price is INDICATIVE and is shown only in the catalogue. The chat
-- assistant never sees it: it may only state fact-sheet facts and never quotes
-- a price for a specific home. The final price is confirmed by the sales team.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE units (
    code                  text PRIMARY KEY,                -- the flat's number, e.g. 'A-904'
    project_id            text NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    bhk                   integer NOT NULL CHECK (bhk IN (2, 3)),
    layout                text NOT NULL CHECK (layout IN ('2bhk', '3bhk')),
    tower                 text NOT NULL,
    floor                 integer NOT NULL CHECK (floor >= 0),
    area_sqft             integer NOT NULL CHECK (area_sqft > 0),
    facing                text NOT NULL,
    indicative_price_lakh integer NOT NULL CHECK (indicative_price_lakh > 0),
    available             boolean NOT NULL DEFAULT true
);
CREATE INDEX units_project_idx ON units (project_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS units;
-- +goose StatementEnd
