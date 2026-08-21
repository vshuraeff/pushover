package mockpushover

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

type ErrorShape string

const (
	ErrorShapeArray  ErrorShape = "array"
	ErrorShapeObject ErrorShape = "object"
)

type AppQuota struct {
	Limit     int   `json:"limit"`
	Remaining int   `json:"remaining"`
	Reset     int64 `json:"reset"`
}

type AppToken struct {
	Token   string   `json:"token"`
	Name    string   `json:"name,omitempty"`
	ID      int64    `json:"id,omitempty"`
	Icon    string   `json:"icon,omitempty"`
	Quota   AppQuota `json:"quota,omitempty"`
	Credits int      `json:"credits,omitempty"`
}

type User struct {
	Key      string `json:"key"`
	Email    string `json:"email,omitempty"`
	Password string `json:"password,omitempty"`
	Secret   string `json:"secret,omitempty"`
}

type TwoFAAccount struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	User     string `json:"user"`
	Secret   string `json:"secret,omitempty"`
	Code     string `json:"code"`
}

type Device struct {
	ID      string `json:"id"`
	User    string `json:"user"`
	Name    string `json:"name"`
	OS      string `json:"os"`
	Secret  string `json:"secret,omitempty"`
	Enabled bool   `json:"enabled"`
}

type GroupMember struct {
	User     string `json:"user"`
	Device   string `json:"device,omitempty"`
	Memo     string `json:"memo,omitempty"`
	Disabled bool   `json:"disabled"`
}

type Group struct {
	Key     string        `json:"key"`
	Name    string        `json:"name"`
	Members []GroupMember `json:"members,omitempty"`
}

type Receipt struct {
	ID              string   `json:"receipt"`
	Token           string   `json:"-"`
	Tag             string   `json:"tag,omitempty"`
	Acknowledged    int      `json:"acknowledged"`
	AcknowledgedAt  int64    `json:"acknowledged_at"`
	AcknowledgedBy  string   `json:"acknowledged_by"`
	LastDeliveredAt int64    `json:"last_delivered_at"`
	ExpiresAt       int64    `json:"expires_at"`
	Expired         int      `json:"expired"`
	CalledBack      int      `json:"called_back"`
	CalledBackAt    int64    `json:"called_back_at"`
	Cancelled       bool     `json:"cancelled"`
	CancelledAt     int64    `json:"cancelled_at,omitempty"`
	CancelledByTag  bool     `json:"cancelled_by_tag"`
	DeviceIDs       []string `json:"-"`
}

type Message struct {
	ID             int64  `json:"id"`
	IDString       string `json:"id_str"`
	UMID           int64  `json:"umid"`
	UMIDString     string `json:"umid_str"`
	Title          string `json:"title"`
	Message        string `json:"message"`
	App            string `json:"app"`
	AID            int64  `json:"aid"`
	AIDString      string `json:"aid_str"`
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
	Attachment     []byte `json:"-"`
}

type Glance struct {
	User    string `json:"user"`
	Device  string `json:"device,omitempty"`
	Title   string `json:"title,omitempty"`
	Text    string `json:"text,omitempty"`
	Subtext string `json:"subtext,omitempty"`
	Count   int    `json:"count,omitempty"`
	Percent int    `json:"percent,omitempty"`
}

type LicenseAssignment struct {
	User  string `json:"user,omitempty"`
	Email string `json:"email,omitempty"`
	OS    string `json:"os,omitempty"`
}

type LicensePool struct {
	Credits  int                 `json:"credits"`
	Assigned []LicenseAssignment `json:"assigned,omitempty"`
}

type TeamMember struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Email         string   `json:"email"`
	Administrator bool     `json:"administrator"`
	Invited       bool     `json:"invited,omitempty"`
	Devices       []Device `json:"devices,omitempty"`
}

type Team struct {
	Token string       `json:"token"`
	Name  string       `json:"name"`
	Users []TeamMember `json:"users,omitempty"`
}

type SeedState struct {
	Tokens        []AppToken             `json:"tokens,omitempty"`
	Users         []User                 `json:"users,omitempty"`
	TwoFAAccounts []TwoFAAccount         `json:"twofa_accounts,omitempty"`
	Devices       []Device               `json:"devices,omitempty"`
	Groups        []Group                `json:"groups,omitempty"`
	Receipts      []Receipt              `json:"receipts,omitempty"`
	Queues        map[string][]Message   `json:"queues,omitempty"`
	Glances       []Glance               `json:"glances,omitempty"`
	Credits       map[string]int         `json:"credits,omitempty"`
	LicensePools  map[string]LicensePool `json:"license_pools,omitempty"`
	Teams         []Team                 `json:"teams,omitempty"`
	CustomSounds  map[string]string      `json:"custom_sounds,omitempty"`
	QuotaNumbers  map[string]AppQuota    `json:"quota_numbers,omitempty"`
	ErrorShape    ErrorShape             `json:"error_shape,omitempty"`
}

type FaultRule struct {
	Method   string `json:"method"`
	PathGlob string `json:"path_glob"`
	Status   int    `json:"status"`
	Body     any    `json:"body"`
	Times    int    `json:"times"`
}

type JournaledRequest struct {
	Method    string              `json:"method"`
	Path      string              `json:"path"`
	Fields    map[string][]string `json:"fields"`
	Headers   http.Header         `json:"headers"`
	Timestamp time.Time           `json:"timestamp"`
}

type loginAccount struct {
	Email       string
	Password    string
	User        string
	Secret      string
	TwoFACode   string
	Requires2FA bool
}

type store struct {
	mu sync.RWMutex

	apps           map[string]AppToken
	users          map[string]User
	accounts       map[string]loginAccount
	devices        map[string]Device
	groups         map[string]Group
	receipts       map[string]Receipt
	queues         map[string][]Message
	highestDeleted map[string]int64
	glances        map[string]Glance
	licenses       map[string]LicensePool
	teams          map[string]Team
	customSounds   map[string]string
	journal        []JournaledRequest
	faults         []FaultRule
	errorShape     ErrorShape
	nextMessageID  map[string]int64
	nextUMID       int64
}

func newStore(seed SeedState) *store {
	s := &store{}
	s.replace(seed)
	return s
}

func (s *store) replace(seed SeedState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replaceLocked(seed)
}

func (s *store) replaceLocked(seed SeedState) {
	s.apps = make(map[string]AppToken, len(seed.Tokens))
	s.users = make(map[string]User, len(seed.Users))
	s.accounts = make(map[string]loginAccount, len(seed.Users)+len(seed.TwoFAAccounts))
	s.devices = make(map[string]Device, len(seed.Devices))
	s.groups = make(map[string]Group, len(seed.Groups))
	s.receipts = make(map[string]Receipt, len(seed.Receipts))
	s.queues = make(map[string][]Message, len(seed.Queues))
	s.highestDeleted = make(map[string]int64)
	s.glances = make(map[string]Glance, len(seed.Glances))
	s.licenses = make(map[string]LicensePool, len(seed.LicensePools))
	s.teams = make(map[string]Team, len(seed.Teams))
	s.customSounds = make(map[string]string, len(seed.CustomSounds))
	s.nextMessageID = make(map[string]int64)
	s.nextUMID = 1
	s.journal = nil
	s.faults = nil
	s.errorShape = seed.ErrorShape
	if s.errorShape != ErrorShapeObject {
		s.errorShape = ErrorShapeArray
	}

	now := time.Now().Unix()
	for i, app := range seed.Tokens {
		if app.Name == "" {
			app.Name = "Mock Pushover"
		}
		if app.ID == 0 {
			app.ID = int64(i + 1)
		}
		if app.Icon == "" {
			app.Icon = "default"
		}
		quota, ok := seed.QuotaNumbers[app.Token]
		if ok {
			app.Quota = quota
		}
		app.Quota = normalizedQuota(app.Quota, now)
		s.apps[app.Token] = app
		pool, ok := seed.LicensePools[app.Token]
		if !ok {
			pool.Credits = app.Credits
		}
		if credits, ok := seed.Credits[app.Token]; ok {
			pool.Credits = credits
		}
		pool.Assigned = append([]LicenseAssignment(nil), pool.Assigned...)
		s.licenses[app.Token] = pool
	}
	for _, user := range seed.Users {
		s.users[user.Key] = user
		if user.Email != "" {
			s.accounts[user.Email] = loginAccount{
				Email: user.Email, Password: user.Password, User: user.Key, Secret: user.Secret,
			}
		}
	}
	for _, account := range seed.TwoFAAccounts {
		secret := account.Secret
		if secret == "" {
			secret = s.users[account.User].Secret
		}
		s.accounts[account.Email] = loginAccount{
			Email: account.Email, Password: account.Password, User: account.User, Secret: secret,
			TwoFACode: account.Code, Requires2FA: true,
		}
	}
	for _, device := range seed.Devices {
		if !device.Enabled {
			device.Enabled = true
		}
		if device.Secret == "" {
			device.Secret = s.users[device.User].Secret
		}
		s.devices[device.ID] = device
	}
	for _, group := range seed.Groups {
		group.Members = append([]GroupMember(nil), group.Members...)
		s.groups[group.Key] = group
	}
	for _, receipt := range seed.Receipts {
		receipt.DeviceIDs = append([]string(nil), receipt.DeviceIDs...)
		s.receipts[receipt.ID] = receipt
	}
	for deviceID, messages := range seed.Queues {
		copied := make([]Message, len(messages))
		for i := range messages {
			copied[i] = cloneMessage(messages[i])
			if copied[i].ID > s.nextMessageID[deviceID] {
				s.nextMessageID[deviceID] = copied[i].ID
			}
			if copied[i].UMID >= s.nextUMID {
				s.nextUMID = copied[i].UMID + 1
			}
		}
		s.queues[deviceID] = copied
	}
	for _, glance := range seed.Glances {
		s.glances[glanceKey(glance.User, glance.Device)] = glance
	}
	for token, pool := range seed.LicensePools {
		pool.Assigned = append([]LicenseAssignment(nil), pool.Assigned...)
		s.licenses[token] = pool
	}
	for token, credits := range seed.Credits {
		pool := s.licenses[token]
		pool.Credits = credits
		s.licenses[token] = pool
	}
	for _, team := range seed.Teams {
		team.Users = cloneTeamMembers(team.Users)
		s.teams[team.Token] = team
	}
	for key, label := range seed.CustomSounds {
		s.customSounds[key] = label
	}
}

func normalizedQuota(quota AppQuota, now int64) AppQuota {
	if quota.Limit == 0 {
		quota.Limit = 10_000
		quota.Remaining = quota.Limit
	}
	if quota.Reset == 0 {
		quota.Reset = now + int64((30 * 24 * time.Hour).Seconds())
	}
	return quota
}

func cloneMessage(message Message) Message {
	message.Attachment = append([]byte(nil), message.Attachment...)
	return message
}

func cloneTeamMembers(members []TeamMember) []TeamMember {
	copied := make([]TeamMember, len(members))
	for i := range members {
		copied[i] = members[i]
		copied[i].Devices = append([]Device(nil), members[i].Devices...)
	}
	return copied
}

func glanceKey(user, device string) string {
	return user + "\x00" + device
}

func cloneSeed(seed SeedState) (SeedState, error) {
	data, err := json.Marshal(seed)
	if err != nil {
		return SeedState{}, err
	}
	var cloned SeedState
	if err := json.Unmarshal(data, &cloned); err != nil {
		return SeedState{}, err
	}
	return cloned, nil
}
