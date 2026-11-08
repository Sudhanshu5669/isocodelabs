package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Unit is one flat in the public catalogue.
type Unit struct {
	Code                string `json:"code"`
	ProjectID           string `json:"projectId"`
	ProjectName         string `json:"projectName"`
	Locality            string `json:"locality"`
	ReraNumber          string `json:"reraNumber"`
	BHK                 int    `json:"bhk"`
	Layout              *string `json:"layout"` // which 3D floor plan the website draws; null = no 3D model yet
	Tower               string `json:"tower"`
	Floor               int    `json:"floor"`
	AreaSqft            int    `json:"areaSqft"`
	Facing              string `json:"facing"`
	IndicativePriceLakh int    `json:"indicativePriceLakh"`
}

const unitColumns = `u.code, u.project_id, p.name, p.locality, p.rera_number, u.bhk, u.layout, u.tower,
	u.floor, u.area_sqft, u.facing, u.indicative_price_lakh`

func scanUnit(sc scanner, u *Unit) error {
	return sc.Scan(&u.Code, &u.ProjectID, &u.ProjectName, &u.Locality, &u.ReraNumber, &u.BHK, &u.Layout,
		&u.Tower, &u.Floor, &u.AreaSqft, &u.Facing, &u.IndicativePriceLakh)
}

// AvailableUnits lists every flat still for sale, by project then unit number.
func (s *Store) AvailableUnits(ctx context.Context) ([]Unit, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+unitColumns+`
		FROM units u JOIN projects p ON p.id = u.project_id
		WHERE u.available
		ORDER BY p.name, u.code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Unit{}
	for rows.Next() {
		var u Unit
		if err := scanUnit(rows, &u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UnitByCode loads one available flat.
//
// Errors: ErrNotFound.
func (s *Store) UnitByCode(ctx context.Context, code string) (Unit, error) {
	var u Unit
	err := scanUnit(s.pool.QueryRow(ctx, `
		SELECT `+unitColumns+`
		FROM units u JOIN projects p ON p.id = u.project_id
		WHERE u.code = $1 AND u.available`, code), &u)
	if errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, ErrNotFound
	}
	return u, err
}
