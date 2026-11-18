package agent

import "regexp"

// A conservative server-side guard supplements model semantics. Other operations
// may legitimately contain tomorrow/need; only completed-action claims are gated.
var uncertainCompletion = regexp.MustCompile(`(?i)(^|[^а-яё])(не|нет|завтра|вчера|надо|нужно|кажется|возможно|наверное|если|планирую|собираюсь|потом|позже|буду)([^а-яё]|$)|[?？]`)

func ValidateIntent(text string, changes []Change) error {
	for _, c := range changes {
		if (c.Operation == "task_complete" || c.Operation == "shopping_buy" || c.Operation == "wish_purchase" || c.Operation == "redemption_deliver") && uncertainCompletion.MatchString(text) {
			return ErrPlan
		}
	}
	return nil
}
