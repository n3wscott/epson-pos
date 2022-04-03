/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"

	"github.com/n3wscott/escpos/pkg/commands"
)

func main() {
	if err := commands.New().Execute(); err != nil {
		fmt.Println("unexpected error:", err.Error())
	}
}
