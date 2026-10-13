-- +goose Up
-- Currencies the app can hold money in. minor_unit is the ISO 4217 number
-- of decimal places, so amounts can be stored as integers of the minor unit.
CREATE TABLE currencies (
    code       CHAR(3)  PRIMARY KEY CHECK (code ~ '^[A-Z]{3}$'),
    minor_unit SMALLINT NOT NULL CHECK (minor_unit BETWEEN 0 AND 4)
);

INSERT INTO currencies (code, minor_unit) VALUES
    ('IDR', 2),
    ('USD', 2),
    ('SGD', 2),
    ('MYR', 2),
    ('EUR', 2),
    ('JPY', 0);

-- +goose Down
DROP TABLE currencies;
