package access

import (
	"ProxyService2/internal/domain"
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

type AccessService struct {
	state atomic.Value
	cache *lru.LRU[string, domain.AccessDecision]
}

type accessState struct {
	settings      domain.AccessSettings
	rules         []domain.AccessRule
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
	rule  domain.AccessRule
}

type rangerRuleEntry struct {
	network net.IPNet
	rule    domain.AccessRule
}

func (e rangerRuleEntry) Network() net.IPNet {
	return e.network
}

func NewAccessService() *AccessService {
	service := &AccessService{}
	service.cache = lru.NewLRU[string, domain.AccessDecision](512, nil, time.Minute)
	service.state.Store(&accessState{
		settings:      domain.AccessSettings{DefaultPolicy: "allow", CaptchaHeader: "X-Captcha-Token"},
		allowPrefixes: cidranger.NewPCTrieRanger(),
		denyPrefixes:  cidranger.NewPCTrieRanger(),
		greyPrefixes:  cidranger.NewPCTrieRanger(),
	})
	return service
}

func (s *AccessService) ApplyConfig(settings domain.AccessSettings) error {
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
		rules:         make([]domain.AccessRule, 0, len(settings.Rules)),
		allowPrefixes: cidranger.NewPCTrieRanger(),
		denyPrefixes:  cidranger.NewPCTrieRanger(),
		greyPrefixes:  cidranger.NewPCTrieRanger(),
	}

	for _, rule := range settings.Rules {
		if err := domain.ValidateAccessRule(rule); err != nil {
			return fmt.Errorf("invalid access rule %s: %w", rule.ID, err)
		}

		if strings.Contains(rule.Value, "-") {
			ipRange, err := parseIPRangeRule(rule)
			if err != nil {
				return err
			}
			switch rule.Type {
			case domain.AccessRuleAllow:
				next.allowRanges = append(next.allowRanges, ipRange)
			case domain.AccessRuleDeny:
				next.denyRanges = append(next.denyRanges, ipRange)
			case domain.AccessRuleGrey:
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
		case domain.AccessRuleAllow:
			_ = next.allowPrefixes.Insert(entry)
		case domain.AccessRuleDeny:
			_ = next.denyPrefixes.Insert(entry)
		case domain.AccessRuleGrey:
			_ = next.greyPrefixes.Insert(entry)
		default:
			return fmt.Errorf("unsupported rule type: %s", rule.Type)
		}
		next.rules = append(next.rules, rule)
	}

	sortRanges(next.allowRanges)
	sortRanges(next.denyRanges)
	sortRanges(next.greyRanges)

	s.cache = lru.NewLRU[string, domain.AccessDecision](settings.CacheSize, nil, settings.CacheTTL)
	s.state.Store(next)
	return nil
}

func (s *AccessService) List() []domain.AccessRule {
	state := s.state.Load().(*accessState)
	result := append([]domain.AccessRule{}, state.rules...)
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

func (s *AccessService) Settings() domain.AccessSettings {
	return s.state.Load().(*accessState).settings
}

func (s *AccessService) Check(ipValue, captchaValue string) domain.AccessDecision {
	state := s.state.Load().(*accessState)
	parsed, err := netip.ParseAddr(ipValue)
	if err != nil {
		return domain.AccessDecision{Allowed: false, Reason: "invalid_ip"}
	}

	cacheKey := parsed.String()
	if cached, ok := s.cache.Get(cacheKey); ok {
		return cached
	}

	if match := findRuleMatch(parsed, state.denyPrefixes, state.denyRanges); match != nil {
		decision := domain.AccessDecision{
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
		decision := domain.AccessDecision{
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
			return domain.AccessDecision{
				Allowed:      true,
				Reason:       "captcha_verified",
				RuleID:       match.ID,
				RuleType:     match.Type,
				MatchedValue: match.Value,
			}
		}
		return domain.AccessDecision{
			Allowed:         false,
			RequiresCaptcha: true,
			Reason:          "captcha_required",
			RuleID:          match.ID,
			RuleType:        match.Type,
			MatchedValue:    match.Value,
		}
	}

	if strings.EqualFold(state.settings.DefaultPolicy, "deny") {
		decision := domain.AccessDecision{Allowed: false, Reason: "default_deny"}
		s.cache.Add(cacheKey, decision)
		return decision
	}

	decision := domain.AccessDecision{Allowed: true, Reason: "default_allow"}
	s.cache.Add(cacheKey, decision)
	return decision
}

func prefixEntry(prefix netip.Prefix, rule domain.AccessRule) (rangerRuleEntry, error) {
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

func parseIPRangeRule(rule domain.AccessRule) (ipRangeRule, error) {
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
	return ipRangeRule{
		start: ipv4ToUint32(start),
		end:   ipv4ToUint32(end),
		rule:  rule,
	}, nil
}

func findRuleMatch(addr netip.Addr, ranger cidranger.Ranger, ranges []ipRangeRule) *domain.AccessRule {
	if addr.Is4() {
		if matched := matchRange(addr, ranges); matched != nil {
			return matched
		}
	}
	entries, err := ranger.ContainingNetworks(net.IP(addr.AsSlice()))
	if err != nil || len(entries) == 0 {
		return nil
	}
	var best *domain.AccessRule
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

func matchRange(addr netip.Addr, ranges []ipRangeRule) *domain.AccessRule {
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
	b := addr.As4()
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}
