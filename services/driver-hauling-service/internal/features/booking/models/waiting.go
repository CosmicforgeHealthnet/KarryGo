package bookingmodels

import (
	"errors"
	"strings"
	"time"
)

// Stops where waiting time is counted.
const (
	WaitingStopPickup  = "PICKUP"
	WaitingStopDropoff = "DROPOFF"
)

// Waiting fee outcomes.
const (
	WaitingFeeNone          = "NONE"
	WaitingFeePending       = "PENDING"
	WaitingFeePaid          = "PAID"
	WaitingFeeOwed          = "OWED"
	WaitingFeeCash          = "CASH"
	WaitingFeeNotApplicable = "NOT_APPLICABLE"
)

// WaitingPolicy is a set of waiting-time rates.
type WaitingPolicy struct {
	ID           string    `json:"id"`
	FreeMinutes  int       `json:"free_minutes"`
	BlockMinutes int       `json:"block_minutes"`
	BlockFeeKobo int64     `json:"block_fee_kobo"`
	CreatedBy    string    `json:"created_by,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// Validate checks the rates are usable.
func (p WaitingPolicy) Validate() error {
	var problems []string
	if p.FreeMinutes < 0 || p.FreeMinutes > 1440 {
		problems = append(problems, "free_minutes must be between 0 and 1440")
	}
	if p.BlockMinutes <= 0 || p.BlockMinutes > 1440 {
		problems = append(problems, "block_minutes must be between 1 and 1440")
	}
	if p.BlockFeeKobo < 0 {
		problems = append(problems, "block_fee_kobo cannot be negative")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

// ChargeableBlocks is how many blocks a wait of `waited` costs: nothing within
// the free time, then every started block counts in full.
func (p WaitingPolicy) ChargeableBlocks(waited time.Duration) int64 {
	if waited <= 0 || p.BlockMinutes <= 0 {
		return 0
	}
	over := waited - time.Duration(p.FreeMinutes)*time.Minute
	if over <= 0 {
		return 0
	}
	block := time.Duration(p.BlockMinutes) * time.Minute
	return int64((over + block - 1) / block)
}

// BookingWaiting is one booking's waiting record: the rates it is charged at
// and when the driver arrived at and left each stop.
type BookingWaiting struct {
	BookingID        string
	Policy           WaitingPolicy
	PickupArrivedAt  *time.Time
	PickupLeftAt     *time.Time
	DropoffArrivedAt *time.Time
	DropoffLeftAt    *time.Time
	FeeKobo          int64
	FeeStatus        string
	FeeReference     string
}

// WaitingStopView is one stop's wait, live while the driver is still there.
type WaitingStopView struct {
	Stop             string     `json:"stop"`
	ArrivedAt        *time.Time `json:"arrived_at,omitempty"`
	LeftAt           *time.Time `json:"left_at,omitempty"`
	WaitedSeconds    int64      `json:"waited_seconds"`
	FreeUntil        *time.Time `json:"free_until,omitempty"`
	ChargeableBlocks int64      `json:"chargeable_blocks"`
	FeeKobo          int64      `json:"fee_kobo"`
	Running          bool       `json:"running"`
}

// WaitingView is what the apps show.
type WaitingView struct {
	BookingID    string            `json:"booking_id"`
	FreeMinutes  int               `json:"free_minutes"`
	BlockMinutes int               `json:"block_minutes"`
	BlockFeeKobo int64             `json:"block_fee_kobo"`
	Stops        []WaitingStopView `json:"stops"`
	TotalFeeKobo int64             `json:"total_fee_kobo"`
	FeeStatus    string            `json:"fee_status"`
}

func (w BookingWaiting) stop(name string, arrived, left *time.Time, now time.Time) WaitingStopView {
	view := WaitingStopView{Stop: name, ArrivedAt: arrived, LeftAt: left}
	if arrived == nil {
		return view
	}
	freeUntil := arrived.Add(time.Duration(w.Policy.FreeMinutes) * time.Minute)
	view.FreeUntil = &freeUntil
	end := now
	if left != nil {
		end = *left
	} else {
		view.Running = true
	}
	waited := end.Sub(*arrived)
	if waited < 0 {
		waited = 0
	}
	view.WaitedSeconds = int64(waited / time.Second)
	view.ChargeableBlocks = w.Policy.ChargeableBlocks(waited)
	view.FeeKobo = view.ChargeableBlocks * w.Policy.BlockFeeKobo
	return view
}

// View computes both stops as of now. A stop the driver has not left yet keeps
// counting, so the apps can show a live timer.
func (w BookingWaiting) View(now time.Time) WaitingView {
	pickup := w.stop(WaitingStopPickup, w.PickupArrivedAt, w.PickupLeftAt, now)
	dropoff := w.stop(WaitingStopDropoff, w.DropoffArrivedAt, w.DropoffLeftAt, now)
	return WaitingView{
		BookingID:    w.BookingID,
		FreeMinutes:  w.Policy.FreeMinutes,
		BlockMinutes: w.Policy.BlockMinutes,
		BlockFeeKobo: w.Policy.BlockFeeKobo,
		Stops:        []WaitingStopView{pickup, dropoff},
		TotalFeeKobo: pickup.FeeKobo + dropoff.FeeKobo,
		FeeStatus:    w.FeeStatus,
	}
}

// FinalFeeKobo is the fee once both stops are closed. A stop the driver never
// left (an exceptional path) is not charged, so a fee can never grow after the
// job has ended.
func (w BookingWaiting) FinalFeeKobo() int64 {
	var total int64
	for _, s := range []struct{ arrived, left *time.Time }{
		{w.PickupArrivedAt, w.PickupLeftAt},
		{w.DropoffArrivedAt, w.DropoffLeftAt},
	} {
		if s.arrived == nil || s.left == nil {
			continue
		}
		total += w.Policy.ChargeableBlocks(s.left.Sub(*s.arrived)) * w.Policy.BlockFeeKobo
	}
	return total
}
