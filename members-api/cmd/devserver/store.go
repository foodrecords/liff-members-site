package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type memberDoc struct {
	Number           int64  `json:"number"`
	Name             string `json:"name"`
	Point            int    `json:"point"`
	TotalEarnedPoint int    `json:"total_earned_point"`
	LastCheckinDate  string `json:"last_checkin_date,omitempty"`
}

type serialDoc struct {
	Point  int    `json:"point"`
	Used   bool   `json:"used"`
	UsedID string `json:"used_id,omitempty"`
}

type rewardCatalogDoc struct {
	Title          string `json:"title"`
	RequiredPoints int    `json:"required_points"`
	Description    string `json:"description,omitempty"`
	Active         bool   `json:"active"`
	ImageURL       string `json:"image_url,omitempty"`
	SortOrder      int    `json:"sort_order"`
}

type memberCouponDoc struct {
	Code           string    `json:"code,omitempty"`
	Title          string    `json:"title"`
	Description    string    `json:"description,omitempty"`
	DiscountAmount int       `json:"discount_amount,omitempty"`
	DiscountLabel  string    `json:"discount_label,omitempty"`
	ExpiresAt      time.Time `json:"expires_at,omitempty"`
	PointMilestone int       `json:"point_milestone,omitempty"`
	RewardID       string    `json:"reward_id,omitempty"`
	PointCost      int       `json:"point_cost,omitempty"`
	IssuedAt       time.Time `json:"issued_at"`
	Used           bool      `json:"used"`
	UsedAt         time.Time `json:"used_at,omitempty"`
	ProductURL     string    `json:"product_url,omitempty"`
	ImageURL       string    `json:"image_url,omitempty"`
}

type dbData struct {
	Members       map[string]memberDoc                  `json:"members"`
	MemberNumbers map[string]string                     `json:"member_numbers"`
	Serials       map[string]serialDoc                  `json:"serials"`
	RewardCatalog map[string]rewardCatalogDoc           `json:"reward_catalog"`
	MemberCoupons map[string]map[string]memberCouponDoc `json:"member_coupons"`
}

const (
	newMemberBonus    = 200
	dailyCheckinPoint = 100
)

func defaultDB() dbData {
	return dbData{
		Members:       make(map[string]memberDoc),
		MemberNumbers: make(map[string]string),
		Serials: map[string]serialDoc{
			"FRTEST0001":   {Point: 3},
			"FRTEST0002":   {Point: 5},
			"FRTEST0003":   {Point: 10},
			"FR1234567890": {Point: 1},
		},
		RewardCatalog: map[string]rewardCatalogDoc{
			"topping_basic": {
				Title:          "基本トッピングチケット",
				RequiredPoints: 200,
				Description:    "店舗スタッフにこの画面をご提示ください。",
				Active:         true,
				SortOrder:      1,
			},
			"topping_deluxe": {
				Title:          "豪華トッピングチケット",
				RequiredPoints: 400,
				Description:    "店舗スタッフにこの画面をご提示ください。",
				Active:         true,
				SortOrder:      2,
			},
			"side_dish": {
				Title:          "おかずチケット",
				RequiredPoints: 750,
				Description:    "店舗スタッフにこの画面をご提示ください。",
				Active:         true,
				SortOrder:      3,
			},
			"bento": {
				Title:          "お弁当チケット",
				RequiredPoints: 1200,
				Description:    "店舗スタッフにこの画面をご提示ください。",
				Active:         true,
				SortOrder:      4,
			},
		},
		MemberCoupons: make(map[string]map[string]memberCouponDoc),
	}
}

type Store struct {
	mu   sync.Mutex
	path string
	data dbData
}

func NewStore(path string) (*Store, error) {
	s := &Store{path: path}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		s.data = defaultDB()
		return s.flush()
	}
	if err != nil {
		return err
	}
	s.data = defaultDB()
	return json.Unmarshal(b, &s.data)
}

func (s *Store) flush() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0644)
}

func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = defaultDB()
	return s.flush()
}

func (s *Store) GetOrCreateMember(userID string) (memberDoc, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if m, ok := s.data.Members[userID]; ok {
		return m, false, nil
	}

	num, err := s.generateUniqueNumber()
	if err != nil {
		return memberDoc{}, false, err
	}
	m := memberDoc{Number: num, Name: userID, Point: newMemberBonus, TotalEarnedPoint: newMemberBonus}
	s.data.Members[userID] = m
	s.data.MemberNumbers[fmt.Sprintf("%06d", num)] = userID
	return m, true, s.flush()
}

func (s *Store) generateUniqueNumber() (int64, error) {
	ra := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 1000; i++ {
		n := int64(ra.Int31n(1000000))
		if _, exists := s.data.MemberNumbers[fmt.Sprintf("%06d", n)]; !exists {
			return n, nil
		}
	}
	return 0, fmt.Errorf("cannot generate unique member number")
}

type addPointResult struct {
	MemberNumber   int64
	GetPoint       int
	NewPoint       int
	NewTotalEarned int
}

func (s *Store) AddPointFromSerial(userID, serialCode string) (*addPointResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	serial, ok := s.data.Serials[serialCode]
	if !ok {
		return nil, errNotFound
	}
	if serial.Used && serialCode != "FR1234567890" {
		return nil, errAlreadyUsed
	}

	member, exists := s.data.Members[userID]
	if !exists {
		num, err := s.generateUniqueNumber()
		if err != nil {
			return nil, err
		}
		member = memberDoc{Number: num, Name: userID}
		s.data.Members[userID] = member
		s.data.MemberNumbers[fmt.Sprintf("%06d", num)] = userID
	}

	prevTotal := effectiveTotalEarned(member)
	member.Point += serial.Point
	member.TotalEarnedPoint = prevTotal + serial.Point
	s.data.Members[userID] = member

	if serialCode != "FR1234567890" {
		serial.Used = true
		serial.UsedID = fmt.Sprintf("%06d", member.Number)
		s.data.Serials[serialCode] = serial
	}

	if err := s.flush(); err != nil {
		return nil, err
	}
	return &addPointResult{
		MemberNumber:   member.Number,
		GetPoint:       serial.Point,
		NewPoint:       member.Point,
		NewTotalEarned: member.TotalEarnedPoint,
	}, nil
}

func (s *Store) AddPointFromCheckin(userID string) (*addPointResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	jst := time.FixedZone("JST", 9*60*60)
	today := time.Now().In(jst).Format("2006-01-02")

	member, exists := s.data.Members[userID]
	if !exists {
		num, err := s.generateUniqueNumber()
		if err != nil {
			return nil, err
		}
		member = memberDoc{Number: num, Name: userID}
		s.data.Members[userID] = member
		s.data.MemberNumbers[fmt.Sprintf("%06d", num)] = userID
	}
	if member.LastCheckinDate == today {
		return nil, errAlreadyCheckedIn
	}

	prevTotal := effectiveTotalEarned(member)
	member.Point += dailyCheckinPoint
	member.TotalEarnedPoint = prevTotal + dailyCheckinPoint
	member.LastCheckinDate = today
	s.data.Members[userID] = member

	if err := s.flush(); err != nil {
		return nil, err
	}
	return &addPointResult{
		MemberNumber:   member.Number,
		GetPoint:       dailyCheckinPoint,
		NewPoint:       member.Point,
		NewTotalEarned: member.TotalEarnedPoint,
	}, nil
}

func (s *Store) GetMemberPoint(userID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Members[userID].Point
}

func (s *Store) GetMemberCoupons(userID string) map[string]memberCouponDoc {
	s.mu.Lock()
	defer s.mu.Unlock()
	coupons := s.data.MemberCoupons[userID]
	if coupons == nil {
		return make(map[string]memberCouponDoc)
	}
	return coupons
}

type rewardItem struct {
	ID  string
	Doc rewardCatalogDoc
}

func (s *Store) GetRewards() []rewardItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]rewardItem, 0, len(s.data.RewardCatalog))
	for id, r := range s.data.RewardCatalog {
		if r.Active {
			items = append(items, rewardItem{ID: id, Doc: r})
		}
	}
	return items
}

type exchangeResult struct {
	CouponID  string
	Title     string
	PointCost int
	NewPoint  int
}

func (s *Store) ExchangeReward(userID, rewardID string) (*exchangeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	reward, ok := s.data.RewardCatalog[rewardID]
	if !ok || !reward.Active {
		return nil, errNotFound
	}

	member, ok := s.data.Members[userID]
	if !ok {
		return nil, errNotFound
	}
	if member.Point < reward.RequiredPoints {
		return nil, errInsufficientPoints
	}

	member.Point -= reward.RequiredPoints
	s.data.Members[userID] = member

	if s.data.MemberCoupons[userID] == nil {
		s.data.MemberCoupons[userID] = make(map[string]memberCouponDoc)
	}
	couponID := fmt.Sprintf("%s_%d", rewardID, time.Now().UnixNano())
	s.data.MemberCoupons[userID][couponID] = memberCouponDoc{
		Title:       reward.Title,
		Description: reward.Description,
		ImageURL:    reward.ImageURL,
		RewardID:    rewardID,
		PointCost:   reward.RequiredPoints,
		IssuedAt:    time.Now(),
		Used:        false,
	}

	if err := s.flush(); err != nil {
		return nil, err
	}
	return &exchangeResult{
		CouponID:  couponID,
		Title:     reward.Title,
		PointCost: reward.RequiredPoints,
		NewPoint:  member.Point,
	}, nil
}

func (s *Store) UseCoupon(userID, couponID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	coupons := s.data.MemberCoupons[userID]
	if coupons == nil {
		return errNotFound
	}
	c, ok := coupons[couponID]
	if !ok {
		return errNotFound
	}
	if c.Used {
		return errAlreadyUsed
	}
	c.Used = true
	c.UsedAt = time.Now()
	coupons[couponID] = c
	return s.flush()
}

func (s *Store) ListMembers() map[string]memberDoc {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]memberDoc, len(s.data.Members))
	for k, v := range s.data.Members {
		out[k] = v
	}
	return out
}

func (s *Store) ListSerials() map[string]serialDoc {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]serialDoc, len(s.data.Serials))
	for k, v := range s.data.Serials {
		out[k] = v
	}
	return out
}

func (s *Store) CreateSerial(code string, point int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Serials[code] = serialDoc{Point: point}
	return s.flush()
}

// effectiveTotalEarned falls back to point for members without total_earned_point set.
func effectiveTotalEarned(m memberDoc) int {
	if m.TotalEarnedPoint == 0 && m.Point > 0 {
		return m.Point
	}
	return m.TotalEarnedPoint
}

var (
	errNotFound           = fmt.Errorf("not found")
	errAlreadyUsed        = fmt.Errorf("already used")
	errAlreadyCheckedIn   = fmt.Errorf("already checked in today")
	errInsufficientPoints = fmt.Errorf("insufficient points")
)
