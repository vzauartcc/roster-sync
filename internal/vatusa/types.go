package vatusa

import (
	"time"
)

type Roster struct {
	IsTestEnv bool         `json:"testing"`
	Data      []Controller `json:"data"`
}

type ACERoster struct {
	Data []ACEMember `json:"data"`
}

type ACEMember struct {
	CID int `json:"cid"`
}

type Facility struct {
	Data FacilityData `json:"data"`
}

type FacilityData struct {
	Facility FacilityDetails `json:"facility"`
}

type FacilityDetails struct {
	Info FacilityInfo `json:"info"`
}

type FacilityInfo struct {
	ATM  int `json:"atm"`
	DATM int `json:"datm"`
	TA   int `json:"ta"`
	EC   int `json:"ec"`
	FE   int `json:"fe"`
	WM   int `json:"wm"`
}

type Role struct {
	ID        int       `json:"id"`
	CID       int       `json:"cid"`
	Facility  string    `json:"facility"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type CertSyncDate time.Time

type Controller struct {
	CID                    int          `json:"cid"`
	FName                  string       `json:"fname"`
	LName                  string       `json:"lname"`
	Email                  string       `json:"email"`
	Facility               string       `json:"facility"`
	Rating                 int          `json:"rating"`
	CreatedAt              time.Time    `json:"created_at"`
	UpdatedAt              time.Time    `json:"updated_at"`
	FlagNeedsBasicTraining bool         `json:"flag_needbasic"`
	FlagTransferOverride   bool         `json:"flag_xfer_override"`
	FacilityJoinDate       time.Time    `json:"facility_join"`
	IsHomeController       bool         `json:"flag_homecontroller"`
	LastActivityTime       time.Time    `json:"lastactivity"`
	BroadcastOptedIn       bool         `json:"flag_broadcastOptedIn"`
	PreventStaffAssign     any          `json:"flag_preventStaffAssign"`
	DiscordID              int          `json:"discord_id"`
	LastCertSyncDate       CertSyncDate `json:"last_cert_sync"`
	NamePrivacyEnabled     bool         `json:"flag_nameprivacy"`
	LastCompetencyDate     string       `json:"last_compentency_date"`
	PromotionEligible      bool         `json:"promotion_eligible"`
	TransferEligible       bool         `json:"transfer_eligible"`
	Roles                  []Role       `json:"roles"`
	RatingShort            string       `json:"rating_short"`
	IsMentor               bool         `json:"is_mentor"`
	IsSupInstructor        bool         `json:"isSupIns"`
	LastPromotionDate      time.Time    `json:"last_promotion"`
	Membership             string       `json:"membership"`
}
