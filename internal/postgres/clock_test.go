package postgres_test

import "time"

// adminZone is the time zone administrations are made in unless told
// otherwise; the server works out "today" in it.
var adminZone = mustZone("Europe/Amsterdam")

func mustZone(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return l
}

// adminNow is now in the administration's time zone, so the dates a test
// sends and expects are the server's dates whatever the machine's time zone,
// also around midnight and at the turn of a month.
func adminNow() time.Time { return time.Now().In(adminZone) }
