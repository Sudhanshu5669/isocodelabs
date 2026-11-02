// Package models holds one Go struct per database table.
//
// Rules used throughout:
//   - One file per table, named after the table.
//   - Field names are Go style (CreatedAt); the `db` tag is the exact column
//     name, which lets pgx fill a struct straight from a query row.
//   - A `json` tag appears only on fields that may be sent to the browser.
//     Secrets use `json:"-"` so they can never leak by accident.
//   - Columns that allow NULL are pointers (*string, *time.Time, *int). A nil
//     pointer means NULL.
//   - UUID columns are plain strings; queries cast them with ::text.
//   - Columns limited by a CHECK constraint get typed constants here, so the
//     allowed values live in one place in Go as well as in the SQL.
//
// Go lesson: a struct tag is the text in backticks after a field. It is
// metadata that other packages read, here for column names and JSON keys.
package models
