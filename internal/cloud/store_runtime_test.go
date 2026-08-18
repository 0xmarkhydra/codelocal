package cloud

import "testing"

func TestStoreMigrationModeDefaultsToStartup(t *testing.T) {
	t.Setenv("CODELOCAL_MIGRATION_MODE", "")
	mode, err := StoreMigrationMode()
	if err != nil {
		t.Fatal(err)
	}
	if mode != StoreMigrationStartup {
		t.Fatalf("mode=%q want %q", mode, StoreMigrationStartup)
	}
}

func TestStoreMigrationModeAcceptsExplicitModes(t *testing.T) {
	for _, want := range []string{StoreMigrationStartup, StoreMigrationExternal, StoreMigrationOnly} {
		t.Run(want, func(t *testing.T) {
			t.Setenv("CODELOCAL_MIGRATION_MODE", want)
			mode, err := StoreMigrationMode()
			if err != nil {
				t.Fatal(err)
			}
			if mode != want {
				t.Fatalf("mode=%q want %q", mode, want)
			}
		})
	}
}

func TestStoreMigrationModeRejectsUnknownValue(t *testing.T) {
	t.Setenv("CODELOCAL_MIGRATION_MODE", "surprise")
	if _, err := StoreMigrationMode(); err == nil {
		t.Fatal("unknown migration mode must fail closed")
	}
}
