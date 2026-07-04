/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package commands

import "testing"

func TestTargetLessSortsIPv4Numerically(t *testing.T) {
	if !targetLess("192.168.86.56:9100", "192.168.86.246:9100") {
		t.Fatal("expected .56 to sort before .246")
	}
	if targetLess("192.168.86.246:9100", "192.168.86.56:9100") {
		t.Fatal("expected .246 to sort after .56")
	}
}
