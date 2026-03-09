package cmd

import "testing"

func TestStartCmd_Flags(t *testing.T) {
	t.Parallel()
	cases := []struct {
		flag     string
		flagType string
		defValue string
	}{
		{"cycles", "int", "0"},
		{"timeout", "int", "0"},
		{"yes", "bool", "false"},
		{"dry-run", "bool", "false"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.flag, func(t *testing.T) {
			t.Parallel()
			f := startCmd.Flags().Lookup(tc.flag)
			if f == nil {
				t.Fatalf("expected --%s flag to be registered on startCmd", tc.flag)
			}
			if f.Value.Type() != tc.flagType {
				t.Errorf("--%s type = %q, want %q", tc.flag, f.Value.Type(), tc.flagType)
			}
			if f.DefValue != tc.defValue {
				t.Errorf("--%s default = %q, want %q", tc.flag, f.DefValue, tc.defValue)
			}
		})
	}
}

func TestStartCmd_RequiresExactlyOneArg(t *testing.T) {
	t.Parallel()
	if err := startCmd.Args(startCmd, []string{}); err == nil {
		t.Error("expected error for zero args")
	}
	if err := startCmd.Args(startCmd, []string{"ENG-1", "ENG-2"}); err == nil {
		t.Error("expected error for two args")
	}
	if err := startCmd.Args(startCmd, []string{"ENG-1"}); err != nil {
		t.Errorf("expected no error for one arg, got: %v", err)
	}
}
