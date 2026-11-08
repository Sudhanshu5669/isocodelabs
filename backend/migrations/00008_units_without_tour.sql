-- Not every flat has a 3D model. A flat whose floor plan has not been modelled
-- yet has no layout (NULL): the website shows its photos instead of the 3D
-- tour and says the model is on its way. The CHECK still allows only the
-- known plans when a layout is set.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE units ALTER COLUMN layout DROP NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE units SET layout = CASE WHEN bhk = 3 THEN '3bhk' ELSE '2bhk' END WHERE layout IS NULL;
ALTER TABLE units ALTER COLUMN layout SET NOT NULL;
-- +goose StatementEnd
