package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

func SendTelegram(token, chat, text string) error {
	url := "https://api.telegram.org/bot" + token + "/sendMessage"
	body := map[string]string{"chat_id": chat, "text": text}
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		buf := make([]byte, 512)
		n, _ := resp.Body.Read(buf)
		return fmt.Errorf("telegram api %d: %s", resp.StatusCode, string(buf[:n]))
	}
	return nil
}
