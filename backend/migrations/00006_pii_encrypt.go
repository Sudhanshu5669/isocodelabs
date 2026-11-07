package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/pressly/goose/v3"

	"leadflow/internal/pii"
)

// DPDP: encrypt personal data at rest (step 2 of 3).
//
// SQL cannot do this step: the keys live only in the app, never in the
// database. So this migration is written in Go. It reads each plaintext value,
// encrypts it with AES-256-GCM into the twin column added by step 1, and
// computes the blind indexes. Down does the reverse.
//
// Go lesson: init() runs automatically when the package is loaded. Here it
// registers the migration with goose, which then runs it in order between the
// .sql files, inside a transaction.
func init() {
	goose.AddMigrationContext(encryptPII, decryptPII)
}

// piiColumn is one plaintext column and its encrypted twin.
type piiColumn struct {
	plain, enc string
	kind       pii.Kind
}

// piiIndex is a blind-index column computed from one plaintext column.
type piiIndex struct {
	column  string // e.g. "phone_bidx"
	sqlType string // "bytea" or "bytea[]"
	from    string // the plaintext column it is computed from
	compute func(k *pii.Keys, plain string) any
}

type piiTable struct {
	name    string
	columns []piiColumn
	indexes []piiIndex
}

var piiTables = []piiTable{
	{
		name:    "users",
		columns: []piiColumn{{"email", "email_enc", pii.Email}, {"name", "name_enc", pii.Name}},
		indexes: []piiIndex{{"email_bidx", "bytea", "email", func(k *pii.Keys, v string) any {
			return k.Index(pii.Email, strings.ToLower(strings.TrimSpace(v)))
		}}},
	},
	{
		name:    "leads",
		columns: []piiColumn{{"name", "name_enc", pii.Name}, {"phone", "phone_enc", pii.Phone}},
		indexes: []piiIndex{
			{"phone_bidx", "bytea", "phone", func(k *pii.Keys, v string) any { return k.Index(pii.Phone, v) }},
			{"name_words", "bytea[]", "name", func(k *pii.Keys, v string) any {
				words := pii.NameWords(v)
				out := make([][]byte, len(words))
				for i, w := range words {
					out[i] = k.Index(pii.NameWord, w)
				}
				return out
			}},
		},
	},
	{name: "lead_touches", columns: []piiColumn{{"note", "note_enc", pii.Text}}},
	{name: "conversations", columns: []piiColumn{{"buyer_name", "buyer_name_enc", pii.Name}, {"summary", "summary_enc", pii.Text}}},
	{name: "messages", columns: []piiColumn{{"content", "content_enc", pii.Text}}},
	{name: "visits", columns: []piiColumn{{"notes", "notes_enc", pii.Text}}},
	{name: "reminders", columns: []piiColumn{{"body", "body_enc", pii.Text}}},
}

// batchSize rows go into one UPDATE, so a remote database is not asked once per row.
const batchSize = 500

func encryptPII(ctx context.Context, tx *sql.Tx) error {
	keys, err := pii.Active()
	if err != nil {
		return fmt.Errorf("%w: set PII_ENCRYPTION_KEYS and PII_INDEX_KEY before migrating", err)
	}
	for _, t := range piiTables {
		var plainCols []string
		for _, c := range t.columns {
			plainCols = append(plainCols, c.plain)
		}
		rows, err := readRows(ctx, tx, t.name, plainCols)
		if err != nil {
			return err
		}

		// Target columns: every encrypted twin, then every blind index.
		var targets []string
		var types []string
		for _, c := range t.columns {
			targets, types = append(targets, c.enc), append(types, "bytea")
		}
		for _, ix := range t.indexes {
			targets, types = append(targets, ix.column), append(types, ix.sqlType)
		}

		out := make([]updateRow, len(rows))
		for i, r := range rows {
			out[i] = updateRow{id: r.id}
			for j, c := range t.columns {
				if r.values[j] == nil {
					out[i].values = append(out[i].values, nil) // NULL stays NULL
					continue
				}
				sealed, err := keys.Encrypt(c.kind, *r.values[j])
				if err != nil {
					return err
				}
				out[i].values = append(out[i].values, sealed)
			}
			for _, ix := range t.indexes {
				src := r.values[indexOf(plainCols, ix.from)]
				if src == nil {
					out[i].values = append(out[i].values, nil)
				} else {
					out[i].values = append(out[i].values, ix.compute(keys, *src))
				}
			}
		}
		if err := writeRows(ctx, tx, t.name, targets, types, out); err != nil {
			return fmt.Errorf("encrypt %s: %w", t.name, err)
		}
	}
	return nil
}

func decryptPII(ctx context.Context, tx *sql.Tx) error {
	keys, err := pii.Active()
	if err != nil {
		return fmt.Errorf("%w: the same keys are needed to decrypt", err)
	}
	for _, t := range piiTables {
		var encCols, plainCols, types []string
		for _, c := range t.columns {
			encCols = append(encCols, c.enc)
			plainCols = append(plainCols, c.plain)
			types = append(types, "text")
		}
		rows, err := readRows(ctx, tx, t.name, encCols)
		if err != nil {
			return err
		}
		out := make([]updateRow, len(rows))
		for i, r := range rows {
			out[i] = updateRow{id: r.id}
			for j, c := range t.columns {
				if r.values[j] == nil {
					out[i].values = append(out[i].values, nil)
					continue
				}
				plain, err := keys.Decrypt(c.kind, []byte(*r.values[j]))
				if err != nil {
					return fmt.Errorf("decrypt %s.%s of %s: %w", t.name, c.enc, r.id, err)
				}
				out[i].values = append(out[i].values, plain)
			}
		}
		if err := writeRows(ctx, tx, t.name, plainCols, types, out); err != nil {
			return fmt.Errorf("decrypt %s: %w", t.name, err)
		}
	}

	// Put back the plaintext schema's own rules (step 1's Down then drops the twins).
	_, err = tx.ExecContext(ctx, `
		ALTER TABLE users ALTER COLUMN email SET NOT NULL, ALTER COLUMN name SET NOT NULL;
		CREATE UNIQUE INDEX IF NOT EXISTS users_email_key ON users (lower(email));
		ALTER TABLE leads ALTER COLUMN name SET NOT NULL, ALTER COLUMN phone SET NOT NULL;
		CREATE UNIQUE INDEX IF NOT EXISTS leads_phone_key ON leads (phone);
		ALTER TABLE messages ALTER COLUMN content SET NOT NULL;`)
	return err
}

type readRow struct {
	id     string
	values []*string // nil = SQL NULL
}

// readRows reads id plus the given columns of every row. bytea columns arrive
// as their raw bytes inside the string.
func readRows(ctx context.Context, tx *sql.Tx, table string, cols []string) ([]readRow, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`SELECT id::text, %s FROM %s`, strings.Join(cols, ", "), table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []readRow
	for rows.Next() {
		r := readRow{values: make([]*string, len(cols))}
		raw := make([]sql.RawBytes, len(cols))
		dest := []any{&r.id}
		for i := range raw {
			dest = append(dest, &raw[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		for i, b := range raw {
			if b != nil { // RawBytes is reused by the next Scan, so copy it now
				s := string(b)
				r.values[i] = &s
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type updateRow struct {
	id     string
	values []any
}

// writeRows updates the given columns of many rows, batchSize rows per statement:
//
//	UPDATE t SET a = v.a, b = v.b
//	FROM (VALUES ($1::uuid, $2::bytea, $3::bytea), (...)) AS v(id, a, b)
//	WHERE t.id = v.id
//
// Only table and column names from piiTables (fixed in this file) are placed
// in the SQL; every value is a bound argument.
func writeRows(ctx context.Context, tx *sql.Tx, table string, cols, types []string, rows []updateRow) error {
	var set []string
	for _, c := range cols {
		set = append(set, fmt.Sprintf("%s = v.%s", c, c))
	}
	for start := 0; start < len(rows); start += batchSize {
		end := min(start+batchSize, len(rows))
		var tuples []string
		var args []any
		for _, r := range rows[start:end] {
			args = append(args, r.id)
			ph := []string{fmt.Sprintf("$%d::uuid", len(args))}
			for i, v := range r.values {
				args = append(args, v)
				ph = append(ph, fmt.Sprintf("$%d::%s", len(args), types[i]))
			}
			tuples = append(tuples, "("+strings.Join(ph, ", ")+")")
		}
		query := fmt.Sprintf(`UPDATE %s AS t SET %s FROM (VALUES %s) AS v(id, %s) WHERE t.id = v.id`,
			table, strings.Join(set, ", "), strings.Join(tuples, ", "), strings.Join(cols, ", "))
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return nil
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}
