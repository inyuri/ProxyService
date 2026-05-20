package access

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/yl2chen/cidranger"
)

type AccessRuleType string

const (
	AccessRuleAllow AccessRuleType = "allow"
	AccessRuleDeny  AccessRuleType = "deny"
	AccessRuleGrey  AccessRuleType = "grey"
)

type AccessRule struct {
	ID          string         `json:"id"`
	Type        AccessRuleType `json:"type"`
	Value       string         `json:"value"`
	Description string         `json:"description"`
	CreatedAt   time.Time      `json:"createdAt"`
}

type AccessSettings struct {
	DefaultPolicy string
	CaptchaHeader string
	CaptchaToken  string
	CacheTTL      time.Duration
	CacheSize     int
	Rules         []AccessRule
}

type AccessDecision struct {
	Allowed         bool           `json:"allowed"`
	RequiresCaptcha bool           `json:"requiresCaptcha"`
	Reason          string         `json:"reason"`
	RuleID          string         `json:"ruleId,omitempty"`
	RuleType        AccessRuleType `json:"ruleType,omitempty"`
	MatchedValue    string         `json:"matchedValue,omitempty"`
}

type AccessService struct {
	state atomic.Value
	cache *lru.LRU[string, AccessDecision]
}

type accessState struct {
	settings      AccessSettings
	rules         []AccessRule
	allowPrefixes cidranger.Ranger
	denyPrefixes  cidranger.Ranger
	greyPrefixes  cidranger.Ranger
	allowRanges   []ipRangeRule
	denyRanges    []ipRangeRule
	greyRanges    []ipRangeRule
}

type ipRangeRule struct {
	start uint32
	end   uint32
	rule  AccessRule
}

type rangerRuleEntry struct {
	network net.IPNet
	rule    AccessRule
}

func (e rangerRuleEntry) Network() net.IPNet {
	return e.network
}

func NewAccessService() *AccessService {
	service := &AccessService{}
	service.cache = lru.NewLRU[string, AccessDecision](512, nil, time.Minute)
	service.state.Store(&accessState{
		settings:      AccessSettings{DefaultPolicy: "allow", CaptchaHeader: "X-Captcha-Token"},
		allowPrefixes: cidranger.NewPCTrieRanger(),
		denyPrefixes:  cidranger.NewPCTrieRanger(),
		greyPrefixes:  cidranger.NewPCTrieRanger(),
	})
	return service
}

func (s *AccessService) ApplyConfig(settings AccessSettings) error {
	if settings.CacheSize <= 0 {
		settings.CacheSize = 512
	}
	if settings.CacheTTL <= 0 {
		settings.CacheTTL = time.Minute
	}
	if settings.CaptchaHeader == "" {
		settings.CaptchaHeader = "X-Captcha-Token"
	}
	if settings.DefaultPolicy == "" {
		settings.DefaultPolicy = "allow"
	}

	next := &accessState{
		settings:      settings,
		rules:         make([]AccessRule, 0, len(settings.Rules)),
		allowPrefixes: cidranger.NewPCTrieRanger(),
		denyPrefixes:  cidranger.NewPCTrieRanger(),
		greyPrefixes:  cidranger.NewPCTrieRanger(),
	}

	for _, rule := range settings.Rules {
		if err := ValidateAccessRule(rule); err != nil {
			return fmt.Errorf("invalid access rule %s: %w", rule.ID, err)
		}

		if strings.Contains(rule.Value, "-") {
			ipRange, err := parseIPRangeRule(rule)
			if err != nil {
				return err
			}
			switch rule.Type {
			case AccessRuleAllow:
				next.allowRanges = append(next.allowRanges, ipRange)
			case AccessRuleDeny:
				next.denyRanges = append(next.denyRanges, ipRange)
			case AccessRuleGrey:
				next.greyRanges = append(next.greyRanges, ipRange)
			}
			next.rules = append(next.rules, rule)
			continue
		}

		prefix, err := normalizePrefix(rule.Value)
		if err != nil {
			return err
		}
		entry, err := prefixEntry(prefix, rule)
		if err != nil {
			return err
		}
		switch rule.Type {
		case AccessRuleAllow:
			_ = next.allowPrefixes.Insert(entry)
		case AccessRuleDeny:
			_ = next.denyPrefixes.Insert(entry)
		case AccessRuleGrey:
			_ = next.greyPrefixes.Insert(entry)
		default:
			return fmt.Errorf("unsupported rule type: %s", rule.Type)
		}
		next.rules = append(next.rules, rule)
	}

	sortRanges(next.allowRanges)
	sortRanges(next.denyRanges)
	sortRanges(next.greyRanges)

	s.cache = lru.NewLRU[string, AccessDecision](settings.CacheSize, nil, settings.CacheTTL)
	s.state.Store(next)
	return nil
}

func (s *AccessService) List() []AccessRule {
	state := s.state.Load().(*accessState)
	result := append([]AccessRule{}, state.rules...)
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

func (s *AccessService) Settings() AccessSettings {
	return s.state.Load().(*accessState).settings
}

func (s *AccessService) Check(ipValue, captchaValue string) AccessDecision {
	state := s.state.Load().(*accessState)
	parsed, err := netip.ParseAddr(ipValue)
	if err != nil {
		return AccessDecision{Allowed: false, Reason: "invalid_ip"}
	}

	cacheKey := parsed.String()
	if cached, ok := s.cache.Get(cacheKey); ok {
		return cached
	}

	if match := findRuleMatch(parsed, state.denyPrefixes, state.denyRanges); match != nil {
		decision := AccessDecision{
			Allowed:      false,
			Reason:       "blacklist",
			RuleID:       match.ID,
			RuleType:     match.Type,
			MatchedValue: match.Value,
		}
		s.cache.Add(cacheKey, decision)
		return decision
	}

	if match := findRuleMatch(parsed, state.allowPrefixes, state.allowRanges); match != nil {
		decision := AccessDecision{
			Allowed:      true,
			Reason:       "whitelist",
			RuleID:       match.ID,
			RuleType:     match.Type,
			MatchedValue: match.Value,
		}
		s.cache.Add(cacheKey, decision)
		return decision
	}

	if match := findRuleMatch(parsed, state.greyPrefixes, state.greyRanges); match != nil {
		if captchaValue != "" && captchaValue == state.settings.CaptchaToken {
			return AccessDecision{
				Allowed:      true,
				Reason:       "captcha_verified",
				RuleID:       match.ID,
				RuleType:     match.Type,
				MatchedValue: match.Value,
			}
		}
		return AccessDecision{
			Allowed:         false,
			RequiresCaptcha: true,
			Reason:          "captcha_required",
			RuleID:          match.ID,
			RuleType:        match.Type,
			MatchedValue:    match.Value,
		}
	}

	if strings.EqualFold(state.settings.DefaultPolicy, "deny") {
		decision := AccessDecision{Allowed: false, Reason: "default_deny"}
		s.cache.Add(cacheKey, decision)
		return decision
	}

	decision := AccessDecision{Allowed: true, Reason: "default_allow"}
	s.cache.Add(cacheKey, decision)
	return decision
}

func ValidateAccessRule(rule AccessRule) error {
	if rule.ID == "" {
		return errors.New("id is required")
	}
	if rule.Value == "" {
		return errors.New("value is required")
	}
	switch rule.Type {
	case AccessRuleAllow, AccessRuleDeny, AccessRuleGrey:
	default:
		return fmt.Errorf("unknown type %q", rule.Type)
	}

	if strings.Contains(rule.Value, "-") {
		_, err := parseIPRangeRule(rule)
		return err
	}

	_, err := normalizePrefix(rule.Value)
	return err
}

func prefixEntry(prefix netip.Prefix, rule AccessRule) (rangerRuleEntry, error) {
	_, network, err := net.ParseCIDR(prefix.String())
	if err != nil {
		return rangerRuleEntry{}, err
	}
	return rangerRuleEntry{network: *network, rule: rule}, nil
}

func normalizePrefix(value string) (netip.Prefix, error) {
	if strings.Contains(value, "/") {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return netip.Prefix{}, err
		}
		return prefix.Masked(), nil
	}

	addr, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Prefix{}, err
	}
	bits := 32
	if addr.Is6() {
		bits = 128
	}
	return netip.PrefixFrom(addr, bits), nil
}

func parseIPRangeRule(rule AccessRule) (ipRangeRule, error) {
	parts := strings.Split(rule.Value, "-")
	if len(parts) != 2 {
		return ipRangeRule{}, fmt.Errorf("invalid ip range %q", rule.Value)
	}

	start, err := netip.ParseAddr(strings.TrimSpace(parts[0]))
	if err != nil {
		return ipRangeRule{}, err
	}
	end, err := netip.ParseAddr(strings.TrimSpace(parts[1]))
	if err != nil {
		return ipRangeRule{}, err
	}
	if !start.Is4() || !end.Is4() {
		return ipRangeRule{}, errors.New("ip ranges are supported only for IPv4")
	}

	startNum := ipv4ToUint32(start)
	endNum := ipv4ToUint32(end)
	if startNum > endNum {
		return ipRangeRule{}, errors.New("range start must be less than or equal to range end")
	}

	return ipRangeRule{start: startNum, end: endNum, rule: rule}, nil
}

func findRuleMatch(addr netip.Addr, ranger cidranger.Ranger, ranges []ipRangeRule) *AccessRule {
	if addr.Is4() {
		if matched := matchRange(addr, ranges); matched != nil {
			return matched
		}
	}

	entries, err := ranger.ContainingNetworks(net.IP(addr.AsSlice()))
	if err != nil || len(entries) == 0 {
		return nil
	}

	var best *AccessRule
	bestMask := -1
	for _, entry := range entries {
		ruleEntry, ok := entry.(rangerRuleEntry)
		if !ok {
			continue
		}
		ones, _ := ruleEntry.network.Mask.Size()
		if ones > bestMask {
			candidate := ruleEntry.rule
			best = &candidate
			bestMask = ones
		}
	}
	return best
}

func matchRange(addr netip.Addr, ranges []ipRangeRule) *AccessRule {
	target := ipv4ToUint32(addr)
	index := sort.Search(len(ranges), func(i int) bool {
		return ranges[i].start > target
	})
	if index > 0 {
		candidate := ranges[index-1]
		if target >= candidate.start && target <= candidate.end {
			rule := candidate.rule
			return &rule
		}
	}
	return nil
}

func sortRanges(ranges []ipRangeRule) {
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].start == ranges[j].start {
			return ranges[i].end < ranges[j].end
		}
		return ranges[i].start < ranges[j].start
	})
}

func ipv4ToUint32(addr netip.Addr) uint32 {
	bytes := addr.As4()
	return uint32(bytes[0])<<24 | uint32(bytes[1])<<16 | uint32(bytes[2])<<8 | uint32(bytes[3])
}
