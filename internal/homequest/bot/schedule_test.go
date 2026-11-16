package bot

import "testing"

func TestScheduleDialogGrammar(t *testing.T) {
	for _, input := range []string{"каждый день 19:00", "вт,чт 19:00", "каждые 3 дня 19:00", "после 3 дней", "без повтора"} {
		r, e := parseSchedule(input)
		if e != nil || r.Mode == "" {
			t.Fatal(input, r, e)
		}
	}
	if _, e := parseSchedule("завтра вечером"); e == nil {
		t.Fatal("ambiguous schedule accepted")
	}
}
