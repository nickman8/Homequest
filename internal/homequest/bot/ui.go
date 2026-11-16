package bot

import "errors"

// UI mode changes only delivery. Domain actions, permissions and state stay identical.
func (b *Bot) ConfigureUI(mode string) error {
	if mode == "" {
		mode = "auto"
	}
	switch mode {
	case "auto", "widgets", "buttons":
		b.UIMode = mode
		return nil
	}
	return errors.New("TELEGRAM_UI must be auto, widgets or buttons")
}

func cardMessage(chat any, card Card) map[string]any {
	buttons := [][]map[string]string{}
	for _, button := range card.Buttons {
		buttons = append(buttons, []map[string]string{{"text": button.Label, "callback_data": button.Action}})
	}
	return map[string]any{"chat_id": chat, "text": card.Title + "\n\n" + card.Text, "reply_markup": map[string]any{"inline_keyboard": buttons}}
}
