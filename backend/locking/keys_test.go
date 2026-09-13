package locking

import "testing"

func TestSeatIDFromKey(t *testing.T) {
	cases := []struct {
		key    string
		prefix string
		want   int
		ok     bool
	}{
		{"hold:42", holdPrefix, 42, true},
		{"hold:1", holdPrefix, 1, true},
		{"booked:7", bookedPrefix, 7, true},
		{"hold:notanumber", holdPrefix, 0, false},
		{"other:5", holdPrefix, 0, false},
		{"hold:", holdPrefix, 0, false},
	}
	for _, c := range cases {
		got, ok := seatIDFromKey(c.key, c.prefix)
		if ok != c.ok || got != c.want {
			t.Errorf("seatIDFromKey(%q,%q) = (%d,%v), want (%d,%v)", c.key, c.prefix, got, ok, c.want, c.ok)
		}
	}
}

func TestKeyFormats(t *testing.T) {
	if holdKey(15) != "hold:15" {
		t.Errorf("holdKey(15) = %q", holdKey(15))
	}
	if bookedKey(15) != "booked:15" {
		t.Errorf("bookedKey(15) = %q", bookedKey(15))
	}
}
