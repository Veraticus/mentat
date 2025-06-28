package signal

import "time"

// Message represents a Signal message with all metadata.
type Message struct {
	ID          string    `json:"id"`
	Sender      string    `json:"sender"`
	Recipient   string    `json:"recipient"`
	Text        string    `json:"text"`
	Timestamp   time.Time `json:"timestamp"`
	IsDelivered bool      `json:"is_delivered"`
	IsRead      bool      `json:"is_read"`
}