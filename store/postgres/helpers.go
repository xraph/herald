package postgres

// jsonbObject guarantees a non-nil map so an empty jsonb object ('{}') is
// stored instead of SQL NULL. The herald_* tables declare jsonb columns as
// NOT NULL DEFAULT '{}', but the DEFAULT never kicks in: grove always lists the
// column in the INSERT, so a nil map reaches pgx (grove's pgdriver is
// pgx-backed) as an explicit NULL and trips the not-null constraint. This only
// bites the pg driver — Mongo has no such constraint and the sqlite store
// json.Marshals into TEXT, where nil becomes the literal "null".
func jsonbObject(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}
