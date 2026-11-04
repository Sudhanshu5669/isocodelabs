package seed

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// sampleUnits are the flats in the public catalogue. Sizes follow the fact
// sheet (2 BHK about 1,085 to 1,150 sq ft, 3 BHK about 1,590 to 1,710 sq ft) and
// prices stay inside the brief's ₹65 lakh to ₹1.2 crore. Unit numbers, floors,
// facings and individual prices are invented sample data.
var sampleUnits = []struct {
	code, project string
	bhk           int
	tower         string
	floor, area   int
	facing        string
	lakh          int
}{
	{"A-904", "project-1", 3, "A", 9, 1640, "East", 108},
	{"A-1203", "project-1", 3, "A", 12, 1710, "East", 119},
	{"A-302", "project-1", 2, "A", 3, 1085, "South", 66},
	{"B-402", "project-1", 2, "B", 4, 1120, "North", 72},
	{"B-1006", "project-1", 2, "B", 10, 1140, "North", 83},
	{"B-1401", "project-1", 3, "B", 14, 1690, "West", 116},
	{"C-306", "project-2", 2, "C", 3, 1085, "West", 67},
	{"C-801", "project-2", 3, "C", 8, 1590, "North", 98},
	{"C-1205", "project-2", 2, "C", 12, 1130, "East", 81},
	{"D-203", "project-2", 3, "D", 2, 1600, "North", 95},
	{"D-504", "project-2", 2, "D", 5, 1150, "East", 78},
	{"D-1102", "project-2", 3, "D", 11, 1680, "South", 112},
}

const insertUnit = `
	INSERT INTO units (code, project_id, bhk, layout, tower, floor, area_sqft, facing, indicative_price_lakh)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	ON CONFLICT (code) DO NOTHING`

func unitArgs(i int) []any {
	u := sampleUnits[i]
	layout := "2bhk"
	if u.bhk == 3 {
		layout = "3bhk"
	}
	return []any{u.code, u.project, u.bhk, layout, u.tower, u.floor, u.area, u.facing, u.lakh}
}

// units is the seeding step that adds the sample flats.
func (s *seeder) units() {
	for i := range sampleUnits {
		s.exec(insertUnit, unitArgs(i)...)
	}
}

// AddUnits adds the sample flats to a database that is already seeded (they
// arrived after the first seed). Flats that already exist are left alone, so
// running it twice is harmless. It returns how many were added.
func AddUnits(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	added := 0
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		for i := range sampleUnits {
			tag, err := tx.Exec(ctx, insertUnit, unitArgs(i)...)
			if err != nil {
				return err
			}
			added += int(tag.RowsAffected())
		}
		return nil
	})
	return added, err
}
