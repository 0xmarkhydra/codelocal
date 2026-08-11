package main

import (
	"bufio"
	"strings"
	"testing"
)

func TestAskYesNoUsesDefaultOnEmptyInput(t *testing.T) {
	got, err := askYesNo(bufio.NewReader(strings.NewReader("\n")), "Browser?", true)
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("empty input should keep the default")
	}
}

func TestAskYesNoAcceptsExplicitNo(t *testing.T) {
	got, err := askYesNo(bufio.NewReader(strings.NewReader("n\n")), "Computer?", true)
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Fatal("explicit no should disable the capability")
	}
}

func TestAskYesNoRepromptsInvalidInput(t *testing.T) {
	got, err := askYesNo(bufio.NewReader(strings.NewReader("maybe\ny\n")), "Browser?", false)
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("valid answer after invalid input should be returned")
	}
}
