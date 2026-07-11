package snmp

import (
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/gosnmp/gosnmp"
)

// Security levels (design decision 8: noAuthNoPriv is rejected).
const (
	SecurityAuthNoPriv = "authNoPriv"
	SecurityAuthPriv   = "authPriv"
)

// USMUser is an SNMPv3 User-based Security Model credential set. Passwords are
// held in memory only (unsealed from the store at load time).
type USMUser struct {
	Username      string
	SecurityLevel string // authNoPriv | authPriv
	AuthProtocol  string // MD5 | SHA | SHA224 | SHA256 | SHA384 | SHA512
	AuthPass      string
	PrivProtocol  string // DES | AES | AES192 | AES256 | AES192C | AES256C
	PrivPass      string
	EngineID      string
}

// Validate enforces the password-protection requirement (decision 8): a user
// must be at least authNoPriv with an auth protocol and passphrase; authPriv
// additionally requires a privacy protocol and passphrase.
func (u USMUser) Validate() error {
	switch u.SecurityLevel {
	case SecurityAuthNoPriv, SecurityAuthPriv:
	default:
		return fmt.Errorf("snmpv3 user %q: security level must be authNoPriv or authPriv (noAuthNoPriv is rejected)", u.Username)
	}
	if u.AuthPass == "" {
		return fmt.Errorf("snmpv3 user %q: auth password is required", u.Username)
	}
	if _, err := authProtocol(u.AuthProtocol); err != nil {
		return fmt.Errorf("snmpv3 user %q: %w", u.Username, err)
	}
	if u.SecurityLevel == SecurityAuthPriv {
		if u.PrivPass == "" {
			return fmt.Errorf("snmpv3 user %q: privacy password is required for authPriv", u.Username)
		}
		if _, err := privProtocol(u.PrivProtocol); err != nil {
			return fmt.Errorf("snmpv3 user %q: %w", u.Username, err)
		}
	}
	return nil
}

// buildUSMTable constructs a gosnmp security-parameters table from the users.
// Invalid users are skipped with an error aggregated for the caller to surface.
func buildUSMTable(users []USMUser) (*gosnmp.SnmpV3SecurityParametersTable, []error) {
	table := gosnmp.NewSnmpV3SecurityParametersTable(gosnmp.NewLogger(log.New(io.Discard, "", 0)))
	var errs []error
	added := 0
	for _, u := range users {
		if err := u.Validate(); err != nil {
			errs = append(errs, err)
			continue
		}
		params, err := u.toParams()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := table.Add(u.Username, params); err != nil {
			errs = append(errs, fmt.Errorf("snmpv3 user %q: add to table: %w", u.Username, err))
			continue
		}
		added++
	}
	if added == 0 {
		return nil, errs
	}
	return table, errs
}

func (u USMUser) toParams() (*gosnmp.UsmSecurityParameters, error) {
	auth, err := authProtocol(u.AuthProtocol)
	if err != nil {
		return nil, err
	}
	p := &gosnmp.UsmSecurityParameters{
		UserName:                 u.Username,
		AuthenticationProtocol:   auth,
		AuthenticationPassphrase: u.AuthPass,
		PrivacyProtocol:          gosnmp.NoPriv,
		AuthoritativeEngineID:    u.EngineID,
	}
	if u.SecurityLevel == SecurityAuthPriv {
		priv, err := privProtocol(u.PrivProtocol)
		if err != nil {
			return nil, err
		}
		p.PrivacyProtocol = priv
		p.PrivacyPassphrase = u.PrivPass
	}
	return p, nil
}

func authProtocol(name string) (gosnmp.SnmpV3AuthProtocol, error) {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "MD5":
		return gosnmp.MD5, nil
	case "SHA":
		return gosnmp.SHA, nil
	case "SHA224":
		return gosnmp.SHA224, nil
	case "SHA256":
		return gosnmp.SHA256, nil
	case "SHA384":
		return gosnmp.SHA384, nil
	case "SHA512":
		return gosnmp.SHA512, nil
	default:
		return 0, fmt.Errorf("unknown auth protocol %q", name)
	}
}

func privProtocol(name string) (gosnmp.SnmpV3PrivProtocol, error) {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "DES":
		return gosnmp.DES, nil
	case "AES":
		return gosnmp.AES, nil
	case "AES192":
		return gosnmp.AES192, nil
	case "AES256":
		return gosnmp.AES256, nil
	case "AES192C":
		return gosnmp.AES192C, nil
	case "AES256C":
		return gosnmp.AES256C, nil
	default:
		return 0, fmt.Errorf("unknown privacy protocol %q", name)
	}
}
