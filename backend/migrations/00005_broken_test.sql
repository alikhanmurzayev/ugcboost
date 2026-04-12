-- +goose Up
SELECT * FROM nonexistent_table_that_does_not_exist;

-- +goose Down
