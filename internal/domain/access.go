package domain

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
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
		return validateIPRange(rule.Value)
	}
	return validatePrefix(rule.Value)
}

func validateIPRange(value string) error {
	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		return fmt.Errorf("invalid ip range %q", value)
	}
	start, err := netip.ParseAddr(strings.TrimSpace(parts[0]))
	if err != nil {
		return err
	}
	end, err := netip.ParseAddr(strings.TrimSpace(parts[1]))
	if err != nil {
		return err
	}
	if !start.Is4() || !end.Is4() {
		return errors.New("ip ranges are supported only for IPv4")
	}
	sb, eb := start.As4(), end.As4()
	startNum := uint32(sb[0])<<24 | uint32(sb[1])<<16 | uint32(sb[2])<<8 | uint32(sb[3])
	endNum := uint32(eb[0])<<24 | uint32(eb[1])<<16 | uint32(eb[2])<<8 | uint32(eb[3])
	if startNum > endNum {
		return errors.New("range start must be less than or equal to range end")
	}
	return nil
}

func validatePrefix(value string) error {
	if strings.Contains(value, "/") {
		_, err := netip.ParsePrefix(value)
		return err
	}
	_, err := netip.ParseAddr(value)
	return err
}
