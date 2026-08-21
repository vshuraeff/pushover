// package openclient implements pushover open client rest flows.
package openclient

// message is a message returned by the pushover open client api.
type Message struct {
	ID             int64  `json:"id"`
	IDStr          string `json:"id_str"`
	UMID           int64  `json:"umid"`
	UMIDStr        string `json:"umid_str"`
	Title          string `json:"title"`
	Message        string `json:"message"`
	App            string `json:"app"`
	AID            int64  `json:"aid"`
	AIDStr         string `json:"aid_str"`
	Icon           string `json:"icon"`
	Date           int64  `json:"date"`
	QueuedDate     int64  `json:"queued_date"`
	DispatchedDate int64  `json:"dispatched_date"`
	Priority       int    `json:"priority"`
	Sound          string `json:"sound"`
	URL            string `json:"url"`
	URLTitle       string `json:"url_title"`
	Acked          int    `json:"acked"`
	Receipt        string `json:"receipt"`
	HTML           int    `json:"html"`
}
