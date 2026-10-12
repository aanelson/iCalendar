package tz

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestWindowsTableDefaults(t *testing.T) {
	for _, tc := range []struct{ windows, iana string }{
		{"Eastern Standard Time", "America/New_York"},
		{"Central Standard Time", "America/Chicago"},
		{"Mountain Standard Time", "America/Denver"},
		{"Pacific Standard Time", "America/Los_Angeles"},
		{"GMT Standard Time", "Europe/London"},
		{"W. Europe Standard Time", "Europe/Berlin"},
		{"Romance Standard Time", "Europe/Paris"},
		{"China Standard Time", "Asia/Shanghai"},
		{"Tokyo Standard Time", "Asia/Tokyo"},
		{"AUS Eastern Standard Time", "Australia/Sydney"},
		{"UTC", "Etc/UTC"},
		// CLDR spells these with retired names; the table must not.
		{"India Standard Time", "Asia/Kolkata"},
		{"Nepal Standard Time", "Asia/Kathmandu"},
		{"Myanmar Standard Time", "Asia/Yangon"},
		{"FLE Standard Time", "Europe/Kyiv"},
		{"Argentina Standard Time", "America/Argentina/Buenos_Aires"},
		{"Greenland Standard Time", "America/Nuuk"},
		{"US Eastern Standard Time", "America/Indiana/Indianapolis"},
	} {
		if got := windowsZones[tc.windows]; got != tc.iana {
			t.Errorf("windowsZones[%q] = %q, want %q", tc.windows, got, tc.iana)
		}
	}
}

func TestWindowsTableByTerritory(t *testing.T) {
	// A caller that knows the region looks it up first and falls back to
	// the territory "001" default.
	lookup := func(windowsID, region string) string {
		if z, ok := windowsZonesByTerritory[windowsID+"/"+region]; ok {
			return z
		}
		return windowsZones[windowsID]
	}
	for _, tc := range []struct{ windows, region, iana string }{
		{"Eastern Standard Time", "CA", "America/Toronto"},
		{"Eastern Standard Time", "US", "America/New_York"},
		{"Eastern Standard Time", "001", "America/New_York"},
		{"Eastern Standard Time", "FR", "America/New_York"}, // no entry: default
		{"W. Europe Standard Time", "NL", "Europe/Amsterdam"},
		{"W. Europe Standard Time", "CH", "Europe/Zurich"},
		{"India Standard Time", "IN", "Asia/Kolkata"},
		{"SE Asia Standard Time", "VN", "Asia/Ho_Chi_Minh"},
		{"West Pacific Standard Time", "FM", "Pacific/Chuuk"},
		{"SA Pacific Standard Time", "CA", "America/Atikokan"},
	} {
		if got := lookup(tc.windows, tc.region); got != tc.iana {
			t.Errorf("lookup(%q, %q) = %q, want %q", tc.windows, tc.region, got, tc.iana)
		}
	}

	territory := regexp.MustCompile(`^[A-Z]{2}$`)
	for key := range windowsZonesByTerritory {
		i := strings.LastIndex(key, "/")
		if i < 0 || !territory.MatchString(key[i+1:]) {
			t.Errorf("windowsZonesByTerritory key %q is not \"WindowsID/TERRITORY\"", key)
			continue
		}
		if _, ok := windowsZones[key[:i]]; !ok {
			t.Errorf("windowsZonesByTerritory key %q: %q has no default in windowsZones", key, key[:i])
		}
	}
}

func TestWindowsTableNoRetiredNames(t *testing.T) {
	// These are tzdb backward links that name no current zone.tab
	// entry: older spellings of a place, or places merged into another zone.
	retired := make(map[string]bool)
	for _, z := range []string{
		"Africa/Asmera", "Africa/Timbuktu",
		"America/Argentina/ComodRivadavia", "America/Atka", "America/Buenos_Aires",
		"America/Catamarca", "America/Coral_Harbour", "America/Cordoba",
		"America/Ensenada", "America/Fort_Wayne", "America/Godthab",
		"America/Indianapolis", "America/Jujuy", "America/Knox_IN",
		"America/Louisville", "America/Mendoza", "America/Montreal",
		"America/Nipigon", "America/Pangnirtung", "America/Porto_Acre",
		"America/Rainy_River", "America/Rosario", "America/Santa_Isabel",
		"America/Shiprock", "America/Thunder_Bay", "America/Virgin",
		"America/Yellowknife",
		"Antarctica/South_Pole",
		"Asia/Ashkhabad", "Asia/Calcutta", "Asia/Choibalsan", "Asia/Chongqing",
		"Asia/Chungking", "Asia/Dacca", "Asia/Harbin", "Asia/Istanbul",
		"Asia/Kashgar", "Asia/Katmandu", "Asia/Macao", "Asia/Rangoon",
		"Asia/Saigon", "Asia/Tel_Aviv", "Asia/Thimbu", "Asia/Ujung_Pandang",
		"Asia/Ulan_Bator",
		"Atlantic/Faeroe", "Atlantic/Jan_Mayen",
		"Australia/ACT", "Australia/Canberra", "Australia/Currie", "Australia/NSW",
		"Europe/Belfast", "Europe/Kiev", "Europe/Nicosia", "Europe/Tiraspol",
		"Europe/Uzhgorod", "Europe/Zaporozhye",
		"Pacific/Enderbury", "Pacific/Johnston", "Pacific/Ponape",
		"Pacific/Samoa", "Pacific/Truk", "Pacific/Yap",
		"US/Central", "US/Eastern", "US/Mountain", "US/Pacific",
		"GMT0", "Greenwich", "UCT", "Universal", "Zulu",
		"Etc/GMT+0", "Etc/GMT-0", "Etc/GMT0", "Etc/Greenwich", "Etc/UCT",
		"Etc/Universal", "Etc/Zulu",
	} {
		retired[z] = true
	}
	check := func(table, key, zone string) {
		switch {
		case zone == "":
			t.Errorf("%s[%q] is empty", table, key)
		case !strings.Contains(zone, "/") && zone != "UTC":
			t.Errorf("%s[%q] = %q, not an Area/Location name", table, key, zone)
		case retired[zone]:
			t.Errorf("%s[%q] = %q, a retired tzdb name", table, key, zone)
		}
	}
	for k, z := range windowsZones {
		check("windowsZones", k, z)
	}
	for k, z := range windowsZonesByTerritory {
		check("windowsZonesByTerritory", k, z)
	}
}

func TestWindowsTableZonesLoad(t *testing.T) {
	if _, err := time.LoadLocation("America/New_York"); err != nil {
		t.Skipf("no tzdata on this machine: %v", err)
	}
	zones := make(map[string]string) // zone -> one key that maps to it
	for k, z := range windowsZones {
		zones[z] = k
	}
	for k, z := range windowsZonesByTerritory {
		zones[z] = k
	}
	for z, k := range zones {
		if _, err := time.LoadLocation(z); err != nil {
			t.Errorf("time.LoadLocation(%q) (for %q): %v", z, k, err)
		}
	}
}
