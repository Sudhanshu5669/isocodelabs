// Package seed fills an empty database with realistic sample data so the app
// never looks empty. Everything here is invented: names, phone numbers and
// facts are placeholders, not real people. The three leads from the brief are
// included exactly as given.
package seed

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"leadflow/internal/auth"
	"leadflow/internal/reminders"
)

// Options controls a seed run.
type Options struct {
	Password string // demo password for every staff account
	Reset    bool   // wipe existing data first
}

// India Standard Time, used for slot times so "11:00" means 11:00 in Ahmedabad.
var ist = time.FixedZone("IST", 5*3600+1800)

// Run seeds the database inside one transaction: either everything is
// written or nothing is.
func Run(ctx context.Context, pool *pgxpool.Pool, opt Options) error {
	var existing int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 && !opt.Reset {
		return errors.New("database already has data; run with -reset to wipe and reseed")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // no-op after a successful Commit

	if opt.Reset {
		_, err := tx.Exec(ctx, `
			TRUNCATE units, source_spend, reminders, visits, visit_slots, messages, conversations,
			         lead_touches, leads, fact_sheet_entries, projects, sessions, users CASCADE;
			ALTER SEQUENCE lead_code_seq RESTART WITH 60000;`)
		if err != nil {
			return err
		}
	}

	s := &seeder{
		ctx:    ctx,
		tx:     tx,
		rng:    mrand.New(mrand.NewPCG(42, 2026)), // fixed seed: same data every run
		now:    time.Now(),
		taken:  map[string]bool{},
		hashed: "",
	}
	if s.hashed, err = auth.HashPassword(opt.Password); err != nil {
		return err
	}

	steps := []func(){s.users, s.projectsAndFacts, s.units, s.futureSlots, s.leads, s.scenarios, s.visits, s.spend}
	for _, step := range steps {
		step()
		if s.err != nil {
			return s.err
		}
	}
	return tx.Commit(ctx)
}

type exec struct{ id, name, email string }

type leadRow struct {
	id, code, name, source, project, wants string
	lang                                   string
	created                                time.Time
	responded, consent, optedOut           bool
	isGiven                                bool
	assigned                               string
}

type slotRow struct {
	id, project, exec string
	start             time.Time
}

// seeder keeps the state shared between seeding steps. s.err is "sticky":
// once a step fails, later calls do nothing and Run reports the first error.
// This avoids an `if err != nil` after every single statement.
type seeder struct {
	ctx    context.Context
	tx     pgx.Tx
	rng    *mrand.Rand
	now    time.Time
	err    error
	hashed string

	headID  string
	execs   []exec
	leadsL  []leadRow
	future  []slotRow
	taken   map[string]bool // "execID|2026-11-01|11" -> already used by a past visit
	reraOf  map[string]string
	nameOf  map[string]string
	visitOf map[string]bool

	scenarioPhone int // numbering for the hand-made scenario leads (scenarios.go)
}

func (s *seeder) exec(sql string, args ...any) {
	if s.err != nil {
		return
	}
	if _, err := s.tx.Exec(s.ctx, sql, args...); err != nil {
		s.err = err
	}
}

func (s *seeder) id(sql string, args ...any) string {
	if s.err != nil {
		return ""
	}
	var id string
	if err := s.tx.QueryRow(s.ctx, sql, args...).Scan(&id); err != nil {
		s.err = err
	}
	return id
}

func (s *seeder) pick(options ...string) string { return options[s.rng.IntN(len(options))] }

// at returns a time on the given day offset (relative to today, in IST) at hh:mm.
func (s *seeder) at(dayOffset, hour, min int) time.Time {
	n := s.now.In(ist)
	return time.Date(n.Year(), n.Month(), n.Day()+dayOffset, hour, min, 0, 0, ist)
}

// ---------------------------------------------------------------------------

func (s *seeder) users() {
	s.headID = s.id(`INSERT INTO users (email, name, role, password_hash)
		VALUES ('head@companya.test', 'Meera Desai', 'head', $1) RETURNING id::text`, s.hashed)

	for _, e := range []exec{
		{name: "Rohan Mehta", email: "rohan@companya.test"},
		{name: "Aisha Khan", email: "aisha@companya.test"},
		{name: "Vikram Trivedi", email: "vikram@companya.test"},
	} {
		e.id = s.id(`INSERT INTO users (email, name, role, password_hash)
			VALUES ($1, $2, 'executive', $3) RETURNING id::text`, e.email, e.name, s.hashed)
		s.execs = append(s.execs, e)
	}
}

func (s *seeder) projectsAndFacts() {
	s.reraOf = map[string]string{"project-1": "RERA-PLACEHOLDER-P1", "project-2": "RERA-PLACEHOLDER-P2"}
	s.nameOf = map[string]string{"project-1": "Project 1", "project-2": "Project 2"}

	// The pin is a placeholder near the centre of Ahmedabad; the client's real
	// coordinates replace it in the admin editor.
	s.exec(`INSERT INTO projects (id, name, rera_number, locality, address, latitude, longitude) VALUES
		('project-1', 'Project 1', $1, 'Ahmedabad', 'Exact address shared by the sales team', 23.0225, 72.5714),
		('project-2', 'Project 2', $2, 'Ahmedabad', 'Exact address shared by the sales team', 23.0425, 72.5214)`,
		s.reraOf["project-1"], s.reraOf["project-2"])

	// The fact sheet is the only source the assistant may quote. Topics that are
	// missing on purpose (payment plans, loans, discounts) make the assistant
	// say "a person will confirm" and hand off.
	facts := []struct{ topic, label, value string }{
		{"configurations", "Apartment types", "2 BHK and 3 BHK apartments."},
		{"area", "Apartment area", "2 BHK apartments are about 1,085 to 1,150 sq ft. 3 BHK apartments are about 1,590 to 1,710 sq ft."},
		{"price_range", "Indicative price range", "Indicative prices across our projects run from ₹65 lakh to ₹1.2 crore. The final price, and any offer, is confirmed by the sales team."},
		{"possession", "Possession", "Expected possession: December 2027."},
		{"amenities", "Amenities", "Clubhouse, gym, children's play area, landscaped garden and 24x7 security."},
		{"parking", "Parking", "One covered parking space with every 2 BHK and two with every 3 BHK."},
		{"location", "Location", "Located in Ahmedabad. The sales executive shares the exact location pin when you book a visit."},
		{"visits", "Site visits", "Site visits are by appointment, in the free slots shown while booking. Each visit lasts about one hour."},
	}
	for _, p := range []string{"project-1", "project-2"} {
		for _, f := range facts {
			s.exec(`INSERT INTO fact_sheet_entries (project_id, topic, label, value, updated_by)
				VALUES ($1, $2, $3, $4, $5::uuid)`, p, f.topic, f.label, f.value, s.headID)
		}
		s.exec(`INSERT INTO fact_sheet_entries (project_id, topic, label, value, updated_by)
			VALUES ($1, 'rera', 'RERA registration', $2, $3::uuid)`,
			p, "RERA registration number: "+s.reraOf[p], s.headID)
	}
}

// execsFor lists the executives who show a given project. Rohan covers Project 1,
// Aisha covers Project 2 and Vikram covers both.
func (s *seeder) execsFor(project string) []exec {
	if project == "project-1" {
		return []exec{s.execs[0], s.execs[2]}
	}
	return []exec{s.execs[1], s.execs[2]}
}

// futureSlots creates one-hour slots for the next two weeks (Monday to Saturday).
func (s *seeder) futureSlots() {
	times := [][2]int{{10, 0}, {11, 30}, {15, 0}, {16, 30}}
	for d := 0; d < 14; d++ {
		if s.at(d, 12, 0).Weekday() == time.Sunday {
			continue
		}
		for _, e := range s.execs {
			project := "project-1"
			switch e.email {
			case "aisha@companya.test":
				project = "project-2"
			case "vikram@companya.test":
				if d%2 == 1 {
					project = "project-2"
				}
			}
			for _, t := range times {
				start := s.at(d, t[0], t[1])
				if start.Before(s.now.Add(3 * time.Hour)) {
					continue // too soon to book
				}
				id := s.id(`INSERT INTO visit_slots (project_id, executive_id, starts_at, ends_at)
					VALUES ($1, $2::uuid, $3, $4) RETURNING id::text`, project, e.id, start, start.Add(time.Hour))
				s.future = append(s.future, slotRow{id: id, project: project, exec: e.id, start: start})
			}
		}
	}
}

var (
	firstNames = []string{"Jignesh", "Pooja", "Rajesh", "Dhruv", "Hetal", "Mitesh", "Komal", "Chirag", "Bhavna",
		"Parth", "Ankita", "Nirav", "Riddhi", "Harsh", "Krupa", "Yash", "Mansi", "Kunal", "Foram", "Sanjay",
		"Disha", "Tushar", "Rinkal", "Alpesh", "Zalak", "Amit", "Neha", "Rahul", "Priya", "Vivek"}
	lastNames = []string{"Patel", "Shah", "Joshi", "Mehta", "Desai", "Trivedi", "Parmar", "Thakkar", "Modi",
		"Gandhi", "Pandya", "Vyas", "Soni", "Bhatt", "Chauhan", "Rana", "Dave", "Amin"}
)

// weighted picks a source with the weights below (portal-heavy, like the brief).
func (s *seeder) source() string {
	r := s.rng.IntN(100)
	switch {
	case r < 38:
		return "property_portal"
	case r < 65:
		return "social_ad"
	case r < 80:
		return "website"
	case r < 92:
		return "hoarding_call"
	default:
		return "manual"
	}
}

func (s *seeder) leads() {
	// The three leads from the brief, exactly as given. Codes, names, sources,
	// projects and wants match; the Visit Status column is created in visits().
	given := []leadRow{
		{code: "LD-58210", name: "Hardik Patel", source: "property_portal", project: "project-1", wants: "3 BHK",
			created: time.Date(2026, 10, 30, 10, 15, 0, 0, ist), responded: true, consent: true, isGiven: true},
		{code: "LD-58211", name: "Nidhi Shah", source: "social_ad", project: "project-2", wants: "2 BHK",
			created: time.Date(2026, 11, 1, 18, 40, 0, 0, ist), responded: true, consent: true, isGiven: true},
		{code: "LD-58224", name: "Kavita Joshi", source: "hoarding_call", project: "project-2", wants: "2 BHK",
			created: time.Date(2026, 10, 20, 12, 5, 0, 0, ist), responded: true, consent: true, isGiven: true},
	}
	reserved := map[string]bool{"LD-58210": true, "LD-58211": true, "LD-58224": true}

	rows := append([]leadRow{}, given...)
	for i := 0; len(rows) < 163; i++ {
		code := fmt.Sprintf("LD-%d", 58200+i)
		if reserved[code] {
			continue
		}
		created := s.now.Add(-time.Duration(s.rng.IntN(60*24*60)) * time.Minute) // within the last 60 days
		r := leadRow{
			code:    code,
			name:    s.pick(firstNames...) + " " + s.pick(lastNames...),
			source:  s.source(),
			project: s.pick("project-1", "project-2"),
			wants:   s.pick("2 BHK", "2 BHK", "3 BHK"),
			created: created,
			consent: s.rng.IntN(100) < 96,
		}
		r.optedOut = r.consent && s.rng.IntN(100) < 3
		// Most leads get an instant reply from the assistant, some wait for a
		// person, and a few are never reached (older ones only count as missed).
		roll := s.rng.IntN(100)
		r.responded = roll < 90 || s.now.Sub(created) < 48*time.Hour
		rows = append(rows, r)
	}

	// Every fifth lead after the first few is a likely duplicate of the one before:
	// same name and project, different phone. These are flagged for review.
	dupOf := map[int]int{}
	for i := 8; i < len(rows); i += 31 {
		rows[i].name, rows[i].project, rows[i].wants = rows[i-1].name, rows[i-1].project, rows[i-1].wants
		dupOf[i] = i - 1
	}

	for i := range rows {
		r := &rows[i]
		phone := fmt.Sprintf("+9190000%05d", 10000+i*37) // obviously fake test numbers
		lang := s.pick("en", "en", "en", "hi", "hi", "gu")
		r.lang = lang

		var firstResp *time.Time
		if r.responded {
			var d time.Duration
			if r.isGiven || s.rng.IntN(100) < 80 {
				d = time.Duration(20+s.rng.IntN(150)) * time.Second // the assistant answers in under 3 minutes
			} else {
				d = time.Duration(2+s.rng.IntN(28)) * time.Hour // a person got to it later
			}
			t := r.created.Add(d)
			if t.After(s.now) {
				t = s.now
			}
			firstResp = &t
		}

		minB, maxB := 65, 90
		if r.wants == "3 BHK" {
			minB, maxB = 95, 120
		}
		status := "new"
		if r.responded {
			status = "engaged"
			if !r.isGiven && s.now.Sub(r.created) > 20*24*time.Hour && s.rng.IntN(100) < 35 {
				status = "lost"
			}
		}

		// Engaged leads are handed to a named executive who covers their project.
		if r.responded && r.consent {
			es := s.execsFor(r.project)
			r.assigned = es[s.rng.IntN(len(es))].id
		}
		var assigned any
		if r.assigned != "" {
			assigned = r.assigned
		}
		var consentAt any
		if r.consent {
			consentAt = r.created
		}
		var optedAt any
		if r.optedOut {
			optedAt = r.created.Add(48 * time.Hour)
		}

		r.id = s.id(`
			INSERT INTO leads (code, name, phone, source, project_id, wants, budget_min_lakh, budget_max_lakh,
			                   language, status, consent, consent_at, opted_out, opted_out_at, assigned_to,
			                   first_response_at, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15::uuid,$16,$17,$17)
			RETURNING id::text`,
			r.code, r.name, phone, r.source, r.project, r.wants, minB, maxB, lang, status,
			r.consent, consentAt, r.optedOut, optedAt, assigned, firstResp, r.created)

		s.exec(`INSERT INTO lead_touches (lead_id, source, project_id, note, created_at)
			VALUES ($1::uuid, $2, $3, 'First enquiry', $4)`, r.id, r.source, r.project, r.created)

		// A handful of people enquire again through another channel. The lead keeps
		// its first source and the repeat enquiry is recorded as a touch.
		if !r.isGiven && s.rng.IntN(100) < 8 {
			other := s.pick("website", "social_ad", "property_portal")
			again := r.created.Add(time.Duration(1+s.rng.IntN(5)) * 24 * time.Hour)
			if other != r.source && again.Before(s.now) { // never a repeat enquiry in the future
				s.exec(`INSERT INTO lead_touches (lead_id, source, project_id, note, created_at)
					VALUES ($1::uuid, $2, $3, 'Enquired again; merged by phone number', $4)`,
					r.id, other, r.project, again)
			}
		}
	}
	for i, j := range dupOf {
		s.exec(`UPDATE leads SET possible_duplicate_of = $1::uuid WHERE id = $2::uuid`, rows[j].id, rows[i].id)
	}
	s.leadsL = rows
}

// visits creates upcoming bookings, past visits (visited or no-show) and the
// reminders for each.
func (s *seeder) visits() {
	s.visitOf = map[string]bool{}

	byCode := map[string]*leadRow{}
	for i := range s.leadsL {
		byCode[s.leadsL[i].code] = &s.leadsL[i]
	}

	// The brief's three leads and their Visit Status.
	// (scenarios.go runs first and keeps clear of these two fixed times.)
	hardik := byCode["LD-58210"] // Booked 2026-11-01 11:00 (the time has passed, so it awaits an outcome)
	s.pastSlotVisit(hardik, time.Date(2026, 11, 1, 11, 0, 0, 0, ist), s.execs[0].id, "booked")
	kavita := byCode["LD-58224"] // Visited
	s.pastSlotVisit(kavita, time.Date(2026, 10, 28, 15, 0, 0, 0, ist), s.execs[1].id, "visited")
	// Nidhi Shah (LD-58211): Not booked. Nothing to create.

	var older, recent []*leadRow
	for i := range s.leadsL {
		l := &s.leadsL[i]
		if l.isGiven || !l.responded || !l.consent || l.optedOut || l.assigned == "" {
			continue
		}
		switch {
		case s.now.Sub(l.created) > 10*24*time.Hour:
			older = append(older, l)
		case s.now.Sub(l.created) < 12*24*time.Hour:
			recent = append(recent, l)
		}
	}
	s.rng.Shuffle(len(older), func(i, j int) { older[i], older[j] = older[j], older[i] })
	s.rng.Shuffle(len(recent), func(i, j int) { recent[i], recent[j] = recent[j], recent[i] })

	// Past visits: 8 shown up, 3 no-shows.
	for i := 0; i < 11 && i < len(older); i++ {
		l := older[i]
		status := "visited"
		if i%4 == 3 {
			status = "no_show"
		}
		es := s.execsFor(l.project)
		e := es[s.rng.IntN(len(es))]
		for try := 0; try < 30; try++ {
			maxDays := int(s.now.Sub(l.created).Hours()/24) - 1
			start := time.Date(l.created.In(ist).Year(), l.created.In(ist).Month(), l.created.In(ist).Day()+1+s.rng.IntN(maxDays),
				[]int{10, 11, 15, 16}[s.rng.IntN(4)], 0, 0, 0, ist)
			if start.Weekday() == time.Sunday || s.taken[key(e.id, start)] {
				continue
			}
			s.pastSlotVisit(l, start, e.id, status)
			break
		}
	}

	// Upcoming bookings: claim real open slots so the slot picker later shows them as taken.
	booked := 0
	for _, l := range recent {
		if booked == 5 {
			break
		}
		for i, sl := range s.future {
			if sl.project == l.project && sl.start.After(s.now.Add(6*time.Hour)) && sl.start.Before(s.now.Add(6*24*time.Hour)) {
				s.future = append(s.future[:i], s.future[i+1:]...)
				s.bookFutureSlot(l, sl)
				booked++
				break
			}
		}
	}
}

func key(exec string, t time.Time) string {
	return exec + "|" + t.In(ist).Format("2006-01-02|15")
}

// pastSlotVisit creates a slot at a fixed time and books a visit in it.
func (s *seeder) pastSlotVisit(l *leadRow, start time.Time, execID, status string) {
	s.taken[key(execID, start)] = true
	slot := s.id(`INSERT INTO visit_slots (project_id, executive_id, starts_at, ends_at, status)
		VALUES ($1, $2::uuid, $3, $4, 'booked') RETURNING id::text`, l.project, execID, start, start.Add(time.Hour))
	var outcome any
	if status != "booked" {
		outcome = start.Add(90 * time.Minute)
	}
	visit := s.id(`INSERT INTO visits (code, lead_id, slot_id, status, outcome_at, created_at)
		VALUES ($1, $2::uuid, $3::uuid, $4, $5, $6) RETURNING id::text`,
		token(), l.id, slot, status, outcome, start.Add(-30*time.Hour))

	leadStatus := map[string]string{"booked": "visit_booked", "visited": "visited", "no_show": "engaged"}[status]
	s.exec(`UPDATE leads SET status = $2, assigned_to = $3::uuid, updated_at = now() WHERE id = $1::uuid`, l.id, leadStatus, execID)
	s.reminders(l, visit, start)
}

func (s *seeder) bookFutureSlot(l *leadRow, sl slotRow) {
	s.exec(`UPDATE visit_slots SET status = 'booked' WHERE id = $1::uuid`, sl.id)
	visit := s.id(`INSERT INTO visits (code, lead_id, slot_id, status, created_at)
		VALUES ($1, $2::uuid, $3::uuid, 'booked', $4) RETURNING id::text`, token(), l.id, sl.id, s.now.Add(-time.Duration(1+s.rng.IntN(20))*time.Hour))
	s.exec(`UPDATE leads SET status = 'visit_booked', assigned_to = $2::uuid, updated_at = now() WHERE id = $1::uuid`, l.id, sl.exec)
	s.reminders(l, visit, sl.start)
}

// reminders adds the two reminders for a visit. Anything already due is marked
// sent; opted-out buyers are skipped. Every message carries the RERA number.
func (s *seeder) reminders(l *leadRow, visitID string, start time.Time) {
	for _, r := range []struct {
		kind string
		due  time.Time
	}{
		{"day_before", start.Add(-24 * time.Hour)},
		{"two_hours_before", start.Add(-2 * time.Hour)},
	} {
		status, body, sentAt, reason := "queued", any(nil), any(nil), any(nil)
		switch {
		case l.optedOut || !l.consent:
			status, reason = "skipped", "opted_out"
		case r.due.Before(s.now):
			// The same text the real reminder sender produces (package reminders).
			status = "sent"
			body = reminders.Render(reminders.Visit{
				Kind: r.kind, Language: langOr(l.lang), FirstName: strings.Fields(l.name)[0],
				ProjectName: s.nameOf[l.project], ReraNumber: s.reraOf[l.project],
				Address:       s.id(`SELECT address FROM projects WHERE id = $1`, l.project),
				ExecutiveName: s.id(`SELECT u.name FROM visits v JOIN visit_slots sl ON sl.id = v.slot_id JOIN users u ON u.id = sl.executive_id WHERE v.id = $1::uuid`, visitID),
				StartsAt:      start,
			})
			sentAt = r.due.Add(40 * time.Second)
		}
		s.exec(`INSERT INTO reminders (visit_id, kind, due_at, status, skip_reason, body, sent_at)
			VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)`, visitID, r.kind, r.due, status, reason, body, sentAt)
	}
}

// spend records marketing spend per source and month. The brief gives ₹1,000
// per lead on average; the per-source rates below average out near that but
// differ, so the dashboard's cost per visit by source has something to compare.
// The current month is left blank for the website, to show a source with no
// spend recorded yet.
func (s *seeder) spend() {
	s.exec(`
		INSERT INTO source_spend (source, month, amount_inr)
		SELECT source, month, n * CASE source WHEN 'property_portal' THEN 1100 WHEN 'social_ad' THEN 900
		                                      WHEN 'website' THEN 600 ELSE 1400 END
		FROM (SELECT source, date_trunc('month', created_at AT TIME ZONE 'Asia/Kolkata')::date AS month, count(*) AS n
		      FROM leads WHERE source <> 'manual' GROUP BY 1, 2) per_month
		WHERE NOT (source = 'website' AND month = date_trunc('month', now() AT TIME ZONE 'Asia/Kolkata')::date)`)
}

func langOr(l string) string {
	if l == "" {
		return "en"
	}
	return l
}

// token returns an unguessable code for a buyer's visit link.
func token() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
