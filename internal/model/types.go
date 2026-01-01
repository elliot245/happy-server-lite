package model

type Account struct {
	ID        string
	PublicKey string
	CreatedAt int64
}

type AuthRequest struct {
	ID                string
	PublicKey         string
	SupportsV2        bool
	Response          string
	ResponseAccountID string
	Token             string
	CreatedAt         int64
	UpdatedAt         int64
}

type Session struct {
	ID                string
	UserID            string
	Tag               string
	Seq               int64
	Metadata          string
	MetadataVersion   int
	AgentState        *string
	AgentStateVersion int
	DataEncryptionKey *string
	Active            bool
	ActiveAt          int64
	CreatedAt         int64
	UpdatedAt         int64
	Deleted           bool
}

type SessionMessage struct {
	ID        string
	SessionID string
	Seq       int64
	Content   string
	CreatedAt int64
	UpdatedAt int64
}

type Machine struct {
	ID                 string
	UserID             string
	Metadata           string
	MetadataVersion    int
	DaemonState        *string
	DaemonStateVersion int
	DataEncryptionKey  *string
	CreatedAt          int64
	UpdatedAt          int64
}
