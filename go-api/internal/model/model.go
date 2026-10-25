// Package model holds the domain types. JSON field names and null behaviour match the Spring
// (Jackson) output exactly, so the React frontend works unchanged.
package model

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

type WaterStatus string

const (
	Municipal          WaterStatus = "MUNICIPAL"
	TankerDependent    WaterStatus = "TANKER_DEPENDENT"
	Mixed              WaterStatus = "MIXED"
	PipelineInProgress WaterStatus = "PIPELINE_IN_PROGRESS"
)

func ParseStatus(s string) (WaterStatus, bool) {
	switch WaterStatus(s) {
	case Municipal, TankerDependent, Mixed, PipelineInProgress:
		return WaterStatus(s), true
	}
	return "", false
}

type ReportUrgency string

const (
	UrgencyUnknown  ReportUrgency = "UNKNOWN"
	UrgencyLow      ReportUrgency = "LOW"
	UrgencyMedium   ReportUrgency = "MEDIUM"
	UrgencyHigh     ReportUrgency = "HIGH"
	UrgencyCritical ReportUrgency = "CRITICAL"
)

func ParseUrgency(s string) (ReportUrgency, bool) {
	switch ReportUrgency(s) {
	case UrgencyUnknown, UrgencyLow, UrgencyMedium, UrgencyHigh, UrgencyCritical:
		return ReportUrgency(s), true
	}
	return "", false
}

type Role string

const (
	RoleAdmin  Role = "ADMIN"
	RoleEditor Role = "EDITOR"
)

// Date is a calendar date serialised as "YYYY-MM-DD" (Java LocalDate).
type Date struct{ time.Time }

const dateLayout = "2006-01-02"

func Today() Date                           { return Date{time.Now().UTC().Truncate(24 * time.Hour)} }
func ParseDate(s string) (Date, error)      { t, err := time.Parse(dateLayout, s); return Date{t}, err }
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.Format(dateLayout)) }
func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return fmt.Errorf("invalid date %q (want YYYY-MM-DD)", s)
	}
	d.Time = t
	return nil
}

// Locality mirrors the Spring entity. Pointers are used for every nullable column so that
// "null" round-trips exactly like Jackson.
type Locality struct {
	ID                    *int64         `json:"id"`
	Name                  string         `json:"name"`
	Ward                  *string        `json:"ward"`
	Status                WaterStatus    `json:"status"`
	Latitude              *float64       `json:"latitude"`
	Longitude             *float64       `json:"longitude"`
	Notes                 *string        `json:"notes"`
	SourceName            *string        `json:"sourceName"`
	SourceURL             *string        `json:"sourceUrl"`
	LastVerified          *Date          `json:"lastVerified"`
	Verified              bool           `json:"verified"`
	SourceURLHealthy      *bool          `json:"sourceUrlHealthy"`
	SourceURLLastChecked  *time.Time     `json:"sourceUrlLastChecked"`
	CitizenDescription    *string        `json:"citizenDescription"`
	ReportedUrgency       *ReportUrgency `json:"reportedUrgency"`
	UrgencyScore          *int           `json:"urgencyScore"`
	UrgencySignals        *string        `json:"urgencySignals"`
	PossibleDuplicateOfID *int64         `json:"possibleDuplicateOfId"`
	DuplicateConfidence   *float64       `json:"duplicateConfidence"`
}

// UnmarshalJSON defaults `verified` to true when absent, as the Java field initialiser does.
func (l *Locality) UnmarshalJSON(b []byte) error {
	type alias Locality
	a := alias{Verified: true}
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	*l = Locality(a)
	return nil
}

func (l Locality) Lat() (float64, bool) {
	if l.Latitude == nil || l.Longitude == nil {
		return 0, false
	}
	return *l.Latitude, true
}

// Validate reproduces the bean-validation rules (@NotBlank name, @NotNull status/lat/lng).
func (l Locality) Validate() error {
	var errs []string
	if strings.TrimSpace(l.Name) == "" {
		errs = append(errs, "name: must not be blank")
	}
	if l.Status == "" {
		errs = append(errs, "status: must not be null")
	} else if _, ok := ParseStatus(string(l.Status)); !ok {
		errs = append(errs, "status: must be one of MUNICIPAL, TANKER_DEPENDENT, MIXED, PIPELINE_IN_PROGRESS")
	}
	if l.Latitude == nil {
		errs = append(errs, "latitude: must not be null")
	}
	if l.Longitude == nil {
		errs = append(errs, "longitude: must not be null")
	}
	return joinErrs(errs)
}

func joinErrs(errs []string) error {
	if len(errs) == 0 {
		return nil
	}
	sort.Strings(errs)
	return fmt.Errorf("%s", strings.Join(errs, "; "))
}

type NearbyLocality struct {
	Locality   Locality `json:"locality"`
	DistanceKm float64  `json:"distanceKm"`
}

type User struct {
	ID           int64  `json:"-"`
	Username     string `json:"-"`
	PasswordHash string `json:"-"`
	Role         Role   `json:"-"`
}

type UserView struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     Role   `json:"role"`
}

func (u User) View() UserView { return UserView{u.ID, u.Username, u.Role} }

type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     Role   `json:"role"`
}

var hasLetter = regexp.MustCompile(`[A-Za-z]`)
var hasDigit = regexp.MustCompile(`\d`)

func (r CreateUserRequest) Validate() error {
	var errs []string
	if strings.TrimSpace(r.Username) == "" {
		errs = append(errs, "username: must not be blank")
	}
	if strings.TrimSpace(r.Password) == "" {
		errs = append(errs, "password: must not be blank")
	} else if len(r.Password) < 8 || !hasLetter.MatchString(r.Password) || !hasDigit.MatchString(r.Password) {
		errs = append(errs, "password: Password must be at least 8 characters and include at least one letter and one number")
	}
	switch r.Role {
	case "":
		errs = append(errs, "role: must not be null")
	case RoleAdmin, RoleEditor:
	default:
		errs = append(errs, "role: must be ADMIN or EDITOR")
	}
	return joinErrs(errs)
}

type AuditLog struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Action       string    `json:"action"`
	Detail       *string   `json:"detail"`
	Success      bool      `json:"success"`
	ErrorMessage *string   `json:"errorMessage"`
	Timestamp    time.Time `json:"timestamp"`
}

// Seed is one row of the first-run locality data.
type Seed struct {
	Name, LastVerified                 string
	Ward, Notes, SourceName, SourceURL *string
	Status                             WaterStatus
	Lat, Lng                           float64
}

func (s Seed) ToLocality() Locality {
	lat, lng := s.Lat, s.Lng
	d, _ := ParseDate(s.LastVerified)
	return Locality{Name: s.Name, Ward: s.Ward, Status: s.Status, Latitude: &lat, Longitude: &lng,
		Notes: s.Notes, SourceName: s.SourceName, SourceURL: s.SourceURL, LastVerified: &d, Verified: true}
}

// ReportSubmittedEvent is the RabbitMQ message body (same JSON shape as the Spring record).
type ReportSubmittedEvent struct {
	LocalityID     int64     `json:"localityId"`
	LocalityName   string    `json:"localityName"`
	ProposedStatus string    `json:"proposedStatus"`
	Notes          *string   `json:"notes"`
	SubmittedAt    time.Time `json:"submittedAt"`
}
