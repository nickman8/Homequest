package assistant

import "testing"

func TestSafeCommandValidation(t *testing.T) {
	candidates := []Candidate{{ID: 1, Title: "Вынести мусор", Version: 2}}
	for _, text := range []string{"Я не выкинул мусор", "Я завтра выкину мусор", "Надо выкинуть мусор", "Мне кажется, мусор уже вынесли", "Я выкинул мусор вчера", "Выкинул мусор и помыл посуду"} {
		cmd := Command{Action: "complete", IDs: []int{1}}
		if e := cmd.Validate(text, candidates); e == nil {
			t.Errorf("unsafe automatic completion: %s", text)
		}
	}
	if e := (&Command{Action: "complete", IDs: []int{1}}).Validate("я выкинул мусор", candidates); e != nil {
		t.Fatal(e)
	}
	if e := (&Command{Action: "complete", IDs: []int{999}}).Validate("я выкинул мусор", candidates); e == nil {
		t.Fatal("foreign id")
	}
	for _, raw := range []string{`{"action":"complete","ids":[1],"family_id":3}`, `{"action":"complete","ids":[1]} trailing`, `null`, `{"action":"sql"}`, `{"action":"draft","draft":{"title":"Мусор","points":-1}}`} {
		if _, e := Decode(raw, "я выкинул мусор", candidates); e == nil {
			t.Fatal("invalid command", raw)
		}
	}
	raw := `{"action":"draft","draft":{"title":"Вынести мусор","points":10,"schedule":{"Mode":"weekdays","Days":["tuesday"],"Hour":19,"Minute":0,"Every":0}}}`
	if _, e := Decode(raw, "Напоминай каждый вторник в семь вечера, 10 баллов", candidates); e != nil {
		t.Fatal(e)
	}
}

func TestShoppingDraftValidation(t *testing.T) {
	raw := `{"action":"shopping_draft","shopping":[{"name":"Молоко","quantity":"2 л","note":""}],"ids":[],"draft":null}`
	if _, e := Decode(raw, "добавь молоко в список покупок", nil); e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{`{"action":"shopping_draft","shopping":[{"name":"","quantity":"","note":""}]}`, `{"action":"complete","ids":[1],"shopping":[{"name":"Молоко"}]}`, `{"action":"shopping_draft","shopping":[{"name":"Молоко","family_id":99}]}`} {
		if _, e := Decode(raw, "добавь молоко", []Candidate{{ID: 1}}); e == nil {
			t.Fatal("unsafe shopping command accepted", raw)
		}
	}
}
