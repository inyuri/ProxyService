package domain_test

import (
	"testing"
	"time"

	"ProxyService2/internal/domain"

	"github.com/stretchr/testify/require"
)

func rule(id, typ, value string) domain.AccessRule {
	return domain.AccessRule{ID: id, Type: domain.AccessRuleType(typ), Value: value, CreatedAt: time.Now()}
}

func TestValidateAccessRule_SingleIP(t *testing.T) {
	require.NoError(t, domain.ValidateAccessRule(rule("1", "allow", "192.168.1.1")))
	require.NoError(t, domain.ValidateAccessRule(rule("2", "deny", "10.0.0.5")))
	require.NoError(t, domain.ValidateAccessRule(rule("3", "grey", "172.16.0.50")))
}

func TestValidateAccessRule_CIDR(t *testing.T) {
	require.NoError(t, domain.ValidateAccessRule(rule("1", "allow", "10.0.0.0/8")))
	require.NoError(t, domain.ValidateAccessRule(rule("2", "deny", "172.16.0.0/12")))
	require.NoError(t, domain.ValidateAccessRule(rule("3", "allow", "192.168.0.0/16")))
}

func TestValidateAccessRule_Range(t *testing.T) {
	require.NoError(t, domain.ValidateAccessRule(rule("1", "allow", "10.0.0.1-10.0.0.100")))
	require.NoError(t, domain.ValidateAccessRule(rule("2", "deny", "192.168.1.1-192.168.1.254")))
}

func TestValidateAccessRule_Range_Equal(t *testing.T) {
	require.NoError(t, domain.ValidateAccessRule(rule("1", "allow", "10.0.0.1-10.0.0.1")))
}

func TestValidateAccessRule_EmptyID(t *testing.T) {
	require.Error(t, domain.ValidateAccessRule(domain.AccessRule{Type: domain.AccessRuleAllow, Value: "1.2.3.4"}))
}

func TestValidateAccessRule_EmptyValue(t *testing.T) {
	require.Error(t, domain.ValidateAccessRule(domain.AccessRule{ID: "1", Type: domain.AccessRuleAllow, Value: ""}))
}

func TestValidateAccessRule_UnknownType(t *testing.T) {
	require.Error(t, domain.ValidateAccessRule(rule("1", "unknown", "1.2.3.4")))
}

func TestValidateAccessRule_InvalidIP(t *testing.T) {
	require.Error(t, domain.ValidateAccessRule(rule("1", "allow", "not-an-ip")))
}

func TestValidateAccessRule_InvalidCIDR(t *testing.T) {
	require.Error(t, domain.ValidateAccessRule(rule("1", "allow", "999.0.0.0/8")))
	require.Error(t, domain.ValidateAccessRule(rule("2", "deny", "10.0.0.0/99")))
}

func TestValidateAccessRule_Range_StartAfterEnd(t *testing.T) {
	require.Error(t, domain.ValidateAccessRule(rule("1", "allow", "10.0.0.100-10.0.0.1")))
}

func TestValidateAccessRule_Range_IPv6NotSupported(t *testing.T) {
	require.Error(t, domain.ValidateAccessRule(rule("1", "deny", "::1-::ff")))
}

func TestValidateAccessRule_Range_BadFormat(t *testing.T) {
	require.Error(t, domain.ValidateAccessRule(rule("1", "allow", "10.0.0.1-bad-end")))
}

func TestValidateAccessRule_Range_BadStart(t *testing.T) {
	require.Error(t, domain.ValidateAccessRule(rule("1", "allow", "bad-start-10.0.0.5")))
}
