-- +goose Up
CREATE TABLE users (
    id UUID PRIMARY KEY,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    email TEXT NOT NULL,
    hashed_password TEXT NOT NULL DEFAULT 'unset',
    token TEXT NOT NULL,
    is_chirpy_red BOOLEAN NOT NULL DEFAULT false
);

-- +goose Down
DROP TABLE users;
