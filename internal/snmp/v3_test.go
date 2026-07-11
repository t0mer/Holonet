package snmp

import "testing"

func TestUSMUserValidate(t *testing.T) {
	cases := []struct {
		name string
		user USMUser
		ok   bool
	}{
		{"authPriv valid", USMUser{Username: "u", SecurityLevel: SecurityAuthPriv, AuthProtocol: "SHA", AuthPass: "authpass12", PrivProtocol: "AES", PrivPass: "privpass12"}, true},
		{"authNoPriv valid", USMUser{Username: "u", SecurityLevel: SecurityAuthNoPriv, AuthProtocol: "SHA256", AuthPass: "authpass12"}, true},
		{"noAuthNoPriv rejected", USMUser{Username: "u", SecurityLevel: "noAuthNoPriv", AuthProtocol: "SHA", AuthPass: "x"}, false},
		{"missing auth pass", USMUser{Username: "u", SecurityLevel: SecurityAuthNoPriv, AuthProtocol: "SHA"}, false},
		{"authPriv missing priv pass", USMUser{Username: "u", SecurityLevel: SecurityAuthPriv, AuthProtocol: "SHA", AuthPass: "authpass12", PrivProtocol: "AES"}, false},
		{"bad auth protocol", USMUser{Username: "u", SecurityLevel: SecurityAuthNoPriv, AuthProtocol: "ROT13", AuthPass: "authpass12"}, false},
	}
	for _, c := range cases {
		err := c.user.Validate()
		if c.ok && err != nil {
			t.Errorf("%s: expected valid, got %v", c.name, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
		}
	}
}

func TestBuildUSMTableSkipsInvalid(t *testing.T) {
	table, errs := buildUSMTable([]USMUser{
		{Username: "good", SecurityLevel: SecurityAuthNoPriv, AuthProtocol: "SHA", AuthPass: "authpass12"},
		{Username: "bad", SecurityLevel: "noAuthNoPriv"},
	})
	if table == nil {
		t.Fatal("expected a table with the one valid user")
	}
	if len(errs) != 1 {
		t.Errorf("expected 1 rejection, got %d", len(errs))
	}
}

func TestBuildUSMTableAllInvalid(t *testing.T) {
	table, errs := buildUSMTable([]USMUser{{Username: "bad", SecurityLevel: "noAuthNoPriv"}})
	if table != nil {
		t.Error("expected nil table when no valid users")
	}
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
}
