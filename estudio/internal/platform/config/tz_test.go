package config

import "testing"

// La zona de la radio debe resolverse aunque el sistema no tenga la base de zonas (Windows).
func TestLimaTimezoneEmbedded(t *testing.T) {
	c := Settings{}
	c.Station.Timezone = "America/Lima"
	if loc := c.Location(); loc.String() != "America/Lima" {
		t.Fatalf("zona = %s", loc)
	}
}
